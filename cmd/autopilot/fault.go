// cmd/autopilot/fault.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// FaultInjector controls Docker containers for chaos testing.
type FaultInjector struct {
	docker *client.Client
}

func NewFaultInjector() (*FaultInjector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &FaultInjector{docker: cli}, nil
}

func (f *FaultInjector) Close() {
	if f.docker != nil {
		f.docker.Close()
	}
}

// findContainer finds a container by name substring (e.g., "bidder", "consumer", "kafka").
func (f *FaultInjector) findContainer(ctx context.Context, nameSubstr string) (string, error) {
	containers, err := f.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, nameSubstr) {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container with name containing %q not found", nameSubstr)
}

// RestartContainer restarts a container and waits for it to come back.
func (f *FaultInjector) RestartContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Restarting container %s (%s)...", nameSubstr, id[:12])
	timeout := 10
	return f.docker.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}

// PauseContainer pauses a container (freezes all processes).
func (f *FaultInjector) PauseContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Pausing container %s (%s)...", nameSubstr, id[:12])
	return f.docker.ContainerPause(ctx, id)
}

// UnpauseContainer resumes a paused container.
func (f *FaultInjector) UnpauseContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Unpausing container %s (%s)...", nameSubstr, id[:12])
	return f.docker.ContainerUnpause(ctx, id)
}

// WaitForHealthy polls a health URL until it responds 200 or timeout.
func WaitForHealthy(url string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	deadline := time.After(timeout)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	for {
		select {
		case <-deadline:
			return time.Since(start), fmt.Errorf("service at %s not healthy after %s", url, timeout)
		default:
			resp, err := httpClient.Get(url + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return time.Since(start), nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
