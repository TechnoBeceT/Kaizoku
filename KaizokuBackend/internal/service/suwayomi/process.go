package suwayomi

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	readySignal  = "You are running Javalin"
	startTimeout = 2 * time.Minute
	stopTimeout  = 5 * time.Second
)

// ProcessManager manages the embedded Suwayomi Java server process.
type ProcessManager struct {
	runtimeDir string
	port       int

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
}

// NewProcessManager creates a new Suwayomi process manager.
func NewProcessManager(runtimeDir string, port int) *ProcessManager {
	return &ProcessManager{
		runtimeDir: runtimeDir,
		port:       port,
	}
}

// Start finds the JAR file and launches the Suwayomi Java process.
// It blocks until the process signals it's ready or the context is cancelled.
func (pm *ProcessManager) Start(ctx context.Context) error {
	suwayomiDir := filepath.Join(pm.runtimeDir, "Suwayomi")

	// Find JAR file
	jarFile, err := findJarFile(suwayomiDir)
	if err != nil {
		return fmt.Errorf("find Suwayomi JAR: %w", err)
	}

	log.Info().Str("jar", jarFile).Msg("found Suwayomi JAR")

	// Ensure tmp directory exists and clean old files
	tmpDir := filepath.Join(suwayomiDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		log.Warn().Err(err).Msg("failed to create tmp directory")
	} else {
		cleanTmpDir(tmpDir, 60*time.Minute)
	}

	// Clean up Chrome singleton lock if present
	chromeLockFile := filepath.Join(suwayomiDir, "webview", "SingletonLock")
	_ = os.Remove(chromeLockFile)

	// Build Java command
	args := []string{
		fmt.Sprintf("-Dsuwayomi.tachidesk.config.server.rootDir=%s", pm.runtimeDir),
		fmt.Sprintf("-Djava.io.tmpdir=%s", tmpDir),
		"-jar", jarFile,
	}

	cmd := exec.CommandContext(ctx, "java", args...)
	cmd.Dir = pm.runtimeDir

	// Capture stdout for ready signal detection
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	log.Info().
		Str("cmd", "java").
		Strs("args", args).
		Msg("starting Suwayomi process")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Suwayomi process: %w", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.mu.Unlock()

	// Forward stderr to logger
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Debug().Str("source", "suwayomi-stderr").Msg(scanner.Text())
		}
	}()

	// Wait for ready signal from stdout
	readyCh := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			log.Debug().Str("source", "suwayomi").Msg(line)

			if strings.Contains(line, readySignal) {
				close(readyCh)
				// Continue reading to prevent pipe stalling
			}
		}
	}()

	// Wait for ready signal or timeout
	select {
	case <-readyCh:
		pm.mu.Lock()
		pm.running = true
		pm.mu.Unlock()
		log.Info().Msg("Suwayomi is ready")
		return nil
	case <-time.After(startTimeout):
		pm.Stop()
		return fmt.Errorf("Suwayomi did not become ready within %s", startTimeout)
	case <-ctx.Done():
		pm.Stop()
		return ctx.Err()
	}
}

// Stop gracefully stops the Suwayomi process.
func (pm *ProcessManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cmd == nil || pm.cmd.Process == nil {
		return
	}

	log.Info().Msg("stopping Suwayomi process")

	// Send interrupt signal (SIGTERM on Unix, graceful on Windows)
	if err := pm.cmd.Process.Signal(os.Interrupt); err != nil {
		log.Warn().Err(err).Msg("failed to send interrupt to Suwayomi, killing")
		_ = pm.cmd.Process.Kill()
		return
	}

	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		_ = pm.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("Suwayomi stopped gracefully")
	case <-time.After(stopTimeout):
		log.Warn().Msg("Suwayomi did not stop in time, killing")
		_ = pm.cmd.Process.Kill()
		_ = pm.cmd.Wait()
	}

	pm.running = false
	pm.cmd = nil
}

// IsRunning returns whether the Suwayomi process is running.
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.running
}

// Wait blocks until the Suwayomi process exits.
func (pm *ProcessManager) Wait() error {
	pm.mu.Lock()
	cmd := pm.cmd
	pm.mu.Unlock()

	if cmd == nil {
		return nil
	}
	return cmd.Wait()
}

// findJarFile searches for the first .jar file in the given directory.
func findJarFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no JAR file found in %s", dir)
}

// cleanTmpDir removes files older than maxAge from the tmp directory.
func cleanTmpDir(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				_ = os.RemoveAll(path)
			} else {
				_ = os.Remove(path)
			}
		}
	}
}
