#!/bin/bash
# Isolated test environment for DSP core services
# Usage: ./scripts/test-env.sh [up|down|migrate|services|verify|all]
#
# Ports:
#   PostgreSQL: 6432    Redis: 7380    ClickHouse: 9124/10001
#   Kafka: 10094
#   API: 9181           Bidder: 9180    Internal: 9182
#   Exchange-Sim: 10090 Frontend: 5000

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

PROJECT_NAME="dsp-test"
COMPOSE_FILE="docker-compose.test.yml"

# Test environment variables
export DB_HOST=localhost
export DB_PORT=6432
export DB_USER=dsp
export DB_PASSWORD=dsp_test_password
export DB_NAME=dsp_test
export REDIS_ADDR=localhost:7380
export REDIS_PASSWORD=dsp_test_password
export KAFKA_BROKERS=localhost:10094
export CLICKHOUSE_ADDR=localhost:10001
export CLICKHOUSE_USER=default
export CLICKHOUSE_PASSWORD=dsp_test_password
export API_PORT=9181
export BIDDER_PORT=9180
export INTERNAL_PORT=9182
export CORS_ALLOWED_ORIGINS=http://localhost:5000
export BIDDER_PUBLIC_URL=http://localhost:9180
export BIDDER_HMAC_SECRET=test-hmac-secret-not-for-production
export BIDDER_URL=http://localhost:9180
export EXCHANGE_SIM_PORT=10090
export ADMIN_TOKEN=test-admin-token
export ENV=development

# Autopilot config
export AUTOPILOT_API_URL=http://localhost:9181
export AUTOPILOT_FRONTEND_URL=http://localhost:5000
export AUTOPILOT_EXCHANGE_SIM_URL=http://localhost:10090
export AUTOPILOT_GRAFANA_URL=

cmd_up() {
    echo "Starting test infrastructure..."
    docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" up -d
    echo "Waiting for services to be healthy..."
    sleep 5
    # Wait for postgres
    for i in $(seq 1 30); do
        if docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" exec -T postgres pg_isready -U dsp > /dev/null 2>&1; then
            echo "PostgreSQL ready"
            break
        fi
        sleep 1
    done
    echo "Test infrastructure up."
    echo ""
    echo "Ports:"
    echo "  PostgreSQL: 6432    Redis: 7380"
    echo "  ClickHouse: 9124/10001    Kafka: 10094"
}

cmd_down() {
    echo "Stopping test infrastructure..."
    docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" down -v
    echo "Test infrastructure stopped and volumes removed."
}

cmd_migrate() {
    echo "Running migrations on test database..."
    for f in migrations/*.sql; do
        echo "  Applying $f..."
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f "$f" -q 2>/dev/null || true
    done

    # ClickHouse migrations
    for f in migrations/002_clickhouse.sql migrations/008_clickhouse_attribution.sql; do
        if [ -f "$f" ]; then
            echo "  Applying $f (ClickHouse)..."
            # ClickHouse HTTP interface
            curl -s "http://localhost:9124/?user=$CLICKHOUSE_USER&password=$CLICKHOUSE_PASSWORD" --data-binary @"$f" || true
        fi
    done
    echo "Migrations complete."
}

cmd_services() {
    echo "Starting application services..."

    # Build first
    rm -f bin/api.exe bin/bidder.exe bin/consumer.exe bin/exchange-sim.exe
    go build -o bin/api ./cmd/api/
    go build -o bin/bidder ./cmd/bidder/
    go build -o bin/consumer ./cmd/consumer/
    go build -o bin/exchange-sim ./cmd/exchange-sim/

    # Start in background
    ./bin/api &
    echo "  API server on :$API_PORT (PID $!)"

    ./bin/bidder &
    echo "  Bidder on :$BIDDER_PORT (PID $!)"

    ./bin/consumer &
    echo "  Consumer (PID $!)"

    ./bin/exchange-sim &
    echo "  Exchange-Sim on :10090 (PID $!)"

    # Frontend
    cd web
    NEXT_PUBLIC_API_URL=http://localhost:$API_PORT PORT=5000 npx next dev -p 5000 &
    echo "  Frontend on :5000 (PID $!)"
    cd ..

    echo ""
    echo "All services started. Use 'kill %1 %2 %3 %4 %5' to stop."
}

cmd_verify() {
    echo "Running autopilot verify..."
    go run ./cmd/autopilot/ verify
}

cmd_all() {
    cmd_up
    cmd_migrate
    cmd_services
    echo ""
    echo "=== Test environment ready ==="
    echo "Run './scripts/test-env.sh verify' to execute autopilot verify"
}

case "${1:-all}" in
    up)       cmd_up ;;
    down)     cmd_down ;;
    migrate)  cmd_migrate ;;
    services) cmd_services ;;
    verify)   cmd_verify ;;
    all)      cmd_all ;;
    *)
        echo "Usage: $0 [up|down|migrate|services|verify|all]"
        echo ""
        echo "  up        Start Docker infrastructure"
        echo "  down      Stop and remove everything"
        echo "  migrate   Run database migrations"
        echo "  services  Build and start Go services + frontend"
        echo "  verify    Run autopilot verify"
        echo "  all       up + migrate + services (full setup)"
        exit 1
        ;;
esac
