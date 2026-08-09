// Package daemon runs the background scan loop and manages its process state.
//
// Nothing here writes to stdout: the daemon may be started from a TUI running
// in the alternate screen, where a stray Println corrupts the display. Status
// goes to the vault's log file, and anything the user should read is returned
// to the caller.
package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
)

// IsRunning reports whether a daemon holds the PID file, and its PID.
// A PID file left behind by a crash is cleaned up here rather than believed.
func IsRunning() (bool, int) {
	data, err := os.ReadFile(config.PidPath())
	if err != nil {
		return false, 0
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	// Signal 0 tests for the process without disturbing it.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = os.Remove(config.PidPath())
		return false, 0
	}
	return true, pid
}

// Run scans on a timer until it is signalled to stop. It blocks.
func Run() error {
	if running, pid := IsRunning(); running {
		return fmt.Errorf("envault daemon already running (PID %d)", pid)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, err := store.NewStore()
	if err != nil {
		return err
	}

	logFile, err := os.OpenFile(config.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)

	if err := writePID(); err != nil {
		return err
	}
	defer func() { _ = os.Remove(config.PidPath()) }()

	logger.Printf("daemon started (PID %d), scanning every %ds", os.Getpid(), cfg.ScanIntervalSecs)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(cfg.ScanIntervalSecs) * time.Second)
	defer ticker.Stop()

	scanOnce(cfg, s, logger)
	for {
		select {
		case <-ticker.C:
			scanOnce(cfg, s, logger)
		case sig := <-stop:
			logger.Printf("received %v, shutting down", sig)
			return nil
		}
	}
}

// Stop signals a running daemon to exit.
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
		return fmt.Errorf("cannot stop daemon (PID %d): %w", pid, err)
	}
	return nil
}

func writePID() error {
	if err := os.MkdirAll(config.VaultDir(), 0700); err != nil {
		return err
	}
	pid := fmt.Sprintf("%d", os.Getpid())
	if err := os.WriteFile(config.PidPath(), []byte(pid), 0600); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	return nil
}

// scanOnce logs only when something changed, so an idle daemon does not grow
// the log file forever.
func scanOnce(cfg *config.Config, s *store.Store, logger *log.Logger) {
	res, err := scanner.ScanDirectories(cfg.WatchDirs, s)
	if err != nil {
		logger.Printf("scan failed: %v", err)
		return
	}
	if res.Backed > 0 {
		logger.Printf("scanned %d files: %d new, %d unchanged", len(res.Found), res.Backed, res.Skipped)
	}
	for _, e := range res.Errors {
		logger.Printf("warning: %v", e)
	}
}
