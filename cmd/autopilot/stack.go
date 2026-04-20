package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5"
)

// TestStackManager boots the local isolated test stack when verify is run
// against an empty workstation. It uses the same ports as scripts/test-env.sh
// but avoids shell-specific glue so Windows can self-heal reliably.
type TestStackManager struct {
	cfg     *AutopilotConfig
	logDir  string
	started []*exec.Cmd
}

func NewTestStackManager(cfg *AutopilotConfig, logDir string) *TestStackManager {
	return &TestStackManager{cfg: cfg, logDir: logDir}
}

func (m *TestStackManager) EnsureRunning(client *DSPClient) error {
	missing := m.missingServices(client)
	if len(missing) == 0 {
		return m.ensureMonitoringStack()
	}

	if _, ok := missing["API"]; ok {
		m.retargetToIsolatedTestStack()
		client.BaseURL = m.cfg.APIURL
		client.AdminToken = m.cfg.AdminToken
		if err := m.ensureInfrastructure(); err != nil {
			return err
		}
		if err := m.applyMigrations(); err != nil {
			return err
		}
	}

	if err := m.ensureAppServices(client); err != nil {
		return err
	}
	if err := m.ensureMonitoringStack(); err != nil {
		return err
	}

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.missingServices(client)) == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("services are still not healthy after timeout: %s", strings.Join(sortedKeys(m.missingServices(client)), ", "))
}

func (m *TestStackManager) Stop() error {
	var firstErr error
	for i := len(m.started) - 1; i >= 0; i-- {
		cmd := m.started[i]
		if cmd == nil || cmd.Process == nil {
			continue
		}
		_ = cmd.Process.Kill()
		_, err := cmd.Process.Wait()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.started = nil
	return firstErr
}

func (m *TestStackManager) missingServices(client *DSPClient) map[string]error {
	services := map[string]string{
		"API":          m.cfg.APIURL,
		"Bidder":       m.cfg.BidderURL,
		"Exchange-Sim": m.cfg.ExchangeSimURL,
	}
	missing := make(map[string]error)
	for name, url := range services {
		if err := client.HealthCheck(url); err != nil {
			missing[name] = err
		}
	}
	return missing
}

func (m *TestStackManager) retargetToIsolatedTestStack() {
	m.cfg.APIURL = "http://localhost:9181"
	m.cfg.AdminURL = "http://localhost:9182"
	m.cfg.BidderURL = "http://localhost:9180"
	m.cfg.FrontendURL = "http://localhost:5000"
	m.cfg.ExchangeSimURL = "http://localhost:10090"
	m.cfg.GrafanaURL = "http://localhost:14100"
	m.cfg.AdminToken = "test-admin-token"
}

func (m *TestStackManager) ensureMonitoringStack() error {
	if m.cfg.GrafanaURL == "" {
		return nil
	}
	if err := waitForHTTP200(m.cfg.GrafanaURL, 3*time.Second); err == nil {
		return nil
	}
	log.Println("[PRE-FLIGHT] Starting monitoring stack (api, bidder, consumer, prometheus, grafana)...")
	if err := runCommandQuiet("", "docker", "compose", "up", "-d", "api", "bidder", "consumer", "prometheus", "grafana"); err != nil {
		return fmt.Errorf("start monitoring stack: %w", err)
	}
	if err := waitForHTTP200(m.cfg.GrafanaURL, 90*time.Second); err != nil {
		return fmt.Errorf("grafana not healthy after startup: %w", err)
	}
	return nil
}

func (m *TestStackManager) ensureInfrastructure() error {
	if portsReachable(6432, 7380, 9124, 10001, 10094) {
		return nil
	}
	log.Println("[PRE-FLIGHT] Starting isolated docker infrastructure...")
	if err := runCommand("", "docker", "compose", "-p", "dsp-test", "-f", "docker-compose.test.yml", "up", "-d"); err != nil {
		return fmt.Errorf("start docker infrastructure: %w", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if portsReachable(6432, 7380, 9124, 10001, 10094) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("docker infrastructure did not become reachable in time")
}

func (m *TestStackManager) applyMigrations() error {
	log.Println("[PRE-FLIGHT] Applying test migrations...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pgConn, err := pgx.Connect(ctx, "postgres://dsp:dsp_test_password@localhost:6432/dsp_test?sslmode=disable")
	if err != nil {
		return fmt.Errorf("connect postgres for migrations: %w", err)
	}
	defer pgConn.Close(ctx)

	files, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	for _, file := range files {
		base := filepath.Base(file)
		if isClickHouseMigration(base) {
			continue
		}
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if _, err := pgConn.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply postgres migration %s: %w", base, err)
		}
	}

	chConn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:10001"},
		Auth: clickhouse.Auth{Database: "default", Username: "default", Password: "dsp_test_password"},
	})
	if err != nil {
		return fmt.Errorf("connect clickhouse for migrations: %w", err)
	}
	defer chConn.Close()
	if err := chConn.Ping(ctx); err != nil {
		return fmt.Errorf("ping clickhouse for migrations: %w", err)
	}
	for _, file := range files {
		base := filepath.Base(file)
		if !isClickHouseMigration(base) {
			continue
		}
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		for _, stmt := range splitSQLStatements(string(sqlBytes)) {
			if err := chConn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("apply clickhouse migration %s: %w", base, err)
			}
		}
	}

	for _, topic := range []string{"dsp.bids", "dsp.impressions", "dsp.billing", "dsp.dead-letter"} {
		if err := runCommandQuiet("", "docker", "compose", "-p", "dsp-test", "-f", "docker-compose.test.yml", "exec", "-T", "kafka",
			"/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "localhost:10094", "--create", "--if-not-exists",
			"--topic", topic, "--partitions", "3", "--replication-factor", "1"); err != nil {
			return fmt.Errorf("ensure kafka topic %s: %w", topic, err)
		}
	}

	return nil
}

func (m *TestStackManager) ensureAppServices(client *DSPClient) error {
	missing := m.missingServices(client)
	if len(missing) == 0 {
		return nil
	}

	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		return fmt.Errorf("create service log dir: %w", err)
	}

	if _, ok := missing["API"]; ok {
		if err := m.ensureBuilt("api", "./cmd/api/"); err != nil {
			return err
		}
		if err := m.startManagedProcess("api", binaryPath("api"), m.serviceEnv()...); err != nil {
			return err
		}
	}

	if _, ok := missing["Bidder"]; ok {
		if err := m.ensureBuilt("bidder", "./cmd/bidder/"); err != nil {
			return err
		}
		if err := m.startManagedProcess("bidder", binaryPath("bidder"), m.serviceEnv()...); err != nil {
			return err
		}
	}

	if _, ok := missing["Exchange-Sim"]; ok {
		if err := m.ensureBuilt("exchange-sim", "./cmd/exchange-sim/"); err != nil {
			return err
		}
		if err := m.startManagedProcess("exchange-sim", binaryPath("exchange-sim"), m.serviceEnv()...); err != nil {
			return err
		}
	}

	// Consumer has no health endpoint, so when we bootstrap the isolated stack
	// we start it alongside the rest of the app services.
	if _, ok := missing["API"]; ok {
		if err := m.ensureBuilt("consumer", "./cmd/consumer/"); err != nil {
			return err
		}
		if err := m.startManagedProcess("consumer", binaryPath("consumer"), m.serviceEnv()...); err != nil {
			return err
		}
	}

	return nil
}

func (m *TestStackManager) ensureBuilt(name, pkg string) error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}
	target := binaryPath(name)
	log.Printf("[PRE-FLIGHT] Building %s...", name)
	return runCommand("", "go", "build", "-o", target, pkg)
}

func (m *TestStackManager) startManagedProcess(name, command string, env ...string) error {
	stdout, err := os.Create(filepath.Join(m.logDir, fmt.Sprintf("%s.out.log", name)))
	if err != nil {
		return fmt.Errorf("create %s stdout log: %w", name, err)
	}
	stderr, err := os.Create(filepath.Join(m.logDir, fmt.Sprintf("%s.err.log", name)))
	if err != nil {
		_ = stdout.Close()
		return fmt.Errorf("create %s stderr log: %w", name, err)
	}

	cmd := exec.Command(command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	m.started = append(m.started, cmd)
	return nil
}

func (m *TestStackManager) serviceEnv() []string {
	return append(os.Environ(),
		"DB_HOST=localhost",
		"DB_PORT=6432",
		"DB_USER=dsp",
		"DB_PASSWORD=dsp_test_password",
		"DB_NAME=dsp_test",
		"REDIS_ADDR=localhost:7380",
		"REDIS_PASSWORD=dsp_test_password",
		"KAFKA_BROKERS=localhost:10094",
		"CLICKHOUSE_ADDR=localhost:10001",
		"CLICKHOUSE_USER=default",
		"CLICKHOUSE_PASSWORD=dsp_test_password",
		"API_PORT=9181",
		"INTERNAL_PORT=9182",
		"BIDDER_PORT=9180",
		"BIDDER_INTERNAL_PORT=8183",
		"BIDDER_PUBLIC_URL=http://localhost:9180",
		"BIDDER_HMAC_SECRET=test-hmac-secret-not-for-production",
		"BIDDER_URL=http://localhost:9180",
		"EXCHANGE_SIM_PORT=10090",
		"CORS_ALLOWED_ORIGINS=http://localhost:5000",
		"ADMIN_TOKEN=test-admin-token",
		"ENV=development",
	)
}

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCommandQuiet(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			_, _ = os.Stderr.Write(output)
		}
		return err
	}
	return nil
}

func portsReachable(ports ...int) bool {
	for _, port := range ports {
		if !canDial(port) {
			return false
		}
	}
	return true
}

func canDial(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func splitSQLStatements(sqlText string) []string {
	lines := strings.Split(strings.ReplaceAll(sqlText, "\r", ""), "\n")
	var builder strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}

	rawStatements := strings.Split(builder.String(), ";")
	var statements []string
	for _, stmt := range rawStatements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed != "" {
			statements = append(statements, trimmed)
		}
	}
	return statements
}

func isClickHouseMigration(name string) bool {
	return name == "002_clickhouse.sql" || name == "008_clickhouse_attribution.sql"
}

func binaryPath(name string) string {
	path := filepath.Join("bin", name)
	if runtime.GOOS == "windows" {
		return path + ".exe"
	}
	return path
}

func sortedKeys(m map[string]error) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
