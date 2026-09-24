package qzone

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var (
	supervisorMu     sync.Mutex
	lastStartAttempt time.Time
)

// EnsureBridge checks if the QZone bridge endpoint is responsive.
// If the target is on localhost/127.0.0.1 and not responding, it attempts to launch
// scripts/start-qzone.sh in the background and waits for it to become healthy.
func EnsureBridge(ctx context.Context, endpoint string, token string, logger *slog.Logger) error {
	client := NewHTTPClient(endpoint, token)
	if ok, _ := client.ProbeStatus(ctx); ok {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid QZone endpoint URL: %w", err)
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}

	isLocal := host == "127.0.0.1" || host == "localhost" || host == "::1" || host == ""
	if !isLocal {
		return fmt.Errorf("QZone bridge at %s is unreachable", endpoint)
	}

	supervisorMu.Lock()
	defer supervisorMu.Unlock()

	// Double check after acquiring lock
	if ok, _ := client.ProbeStatus(ctx); ok {
		return nil
	}

	// Avoid spamming start attempts
	if time.Since(lastStartAttempt) < 15*time.Second {
		for i := 0; i < 5; i++ {
			time.Sleep(1 * time.Second)
			if ok, _ := client.ProbeStatus(ctx); ok {
				return nil
			}
		}
	}

	scriptPath := findStartScript()
	if scriptPath == "" {
		return fmt.Errorf("QZone bridge is offline and start-qzone.sh script could not be located")
	}

	if logger != nil {
		logger.Info("Starting QZone bridge supervisor", "script", scriptPath)
	}

	lastStartAttempt = time.Now()
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Dir = filepath.Dir(filepath.Dir(scriptPath))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch QZone bridge via %s: %w", scriptPath, err)
	}
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			if ok, _ := client.ProbeStatus(ctx); ok {
				if logger != nil {
					logger.Info("QZone bridge successfully started and verified online")
				}
				return nil
			}
		}
	}

	return fmt.Errorf("QZone bridge was launched but did not report healthy status within 12s")
}

func findStartScript() string {
	candidates := []string{
		"scripts/start-qzone.sh",
		"../scripts/start-qzone.sh",
		"../../scripts/start-qzone.sh",
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "scripts/start-qzone.sh"),
			filepath.Join(execDir, "../scripts/start-qzone.sh"),
			filepath.Join(execDir, "../../scripts/start-qzone.sh"),
		)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
			return path
		}
	}
	return ""
}
