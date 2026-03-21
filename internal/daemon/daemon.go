package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/akhshy/envault/internal/config"
	"github.com/akhshy/envault/internal/scanner"
	"github.com/akhshy/envault/internal/store"
)

// PidFilePath returns the path to the daemon's PID file.
func PidFilePath() string {
	return filepath.Join(config.VaultDir(), "envault.pid")
}

// LogFilePath returns the path to the daemon's log file.
func LogFilePath() string {
	return filepath.Join(config.VaultDir(), "envault.log")
}

// IsRunning checks if the daemon is already running.
func IsRunning() (bool, int) {
	data, err := os.ReadFile(PidFilePath())
	if err != nil {
		return false, 0
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false, 0
	}
	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	// On Unix, sending signal 0 checks if the process exists
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist, clean up stale PID file
		os.Remove(PidFilePath())
		return false, 0
	}
	return true, pid
}

// writePID writes the current process PID to the PID file.
func writePID() error {
	if err := os.MkdirAll(config.VaultDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(PidFilePath(), []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
}

// removePID removes the PID file.
func removePID() {
	os.Remove(PidFilePath())
}

// Run starts the daemon loop that periodically scans for .env files.
func Run() error {
	if running, pid := IsRunning(); running {
		return fmt.Errorf("envault daemon already running (PID %d)", pid)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	s, err := store.NewStore()
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}

	// Setup logging
	if err := os.MkdirAll(config.VaultDir(), 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(LogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)

	if err := writePID(); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer removePID()

	logger.Println("envault daemon started")
	fmt.Println("envault daemon started (PID", os.Getpid(), ")")

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	interval := time.Duration(cfg.ScanIntervalSecs) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial scan immediately
	runScan(cfg, s, logger)

	for {
		select {
		case <-ticker.C:
			runScan(cfg, s, logger)
		case sig := <-sigCh:
			logger.Printf("received signal %v, shutting down", sig)
			fmt.Println("\nenvault daemon stopped")
			return nil
		}
	}
}

func runScan(cfg *config.Config, s *store.Store, logger *log.Logger) {
	result, err := scanner.ScanDirectories(cfg.WatchDirs, s)
	if err != nil {
		logger.Printf("scan error: %v", err)
		return
	}
	if result.Backed > 0 {
		logger.Printf("scan complete: %d files found, %d new snapshots, %d unchanged",
			len(result.Found), result.Backed, result.Skipped)
	}
	for _, e := range result.Errors {
		logger.Printf("scan warning: %v", e)
	}
}

// Stop sends a termination signal to the running daemon.
func Stop() error {
	running, pid := IsRunning()
	if !running {
		return fmt.Errorf("envault daemon is not running")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon (PID %d): %w", pid, err)
	}
	fmt.Printf("envault daemon stopped (PID %d)\n", pid)
	return nil
}
