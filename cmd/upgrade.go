package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and build info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("envault — your .env files, safely vaulted")
		fmt.Println()
		fmt.Printf("  Version:  %s\n", Version)
		fmt.Printf("  Built:    %s\n", BuildDate)
		fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Println()
		fmt.Println("  Created by Akhshy (Code_Wid_Mapla)")
		fmt.Println("  https://github.com/akhshyganesh/envault")
		fmt.Println("  https://www.youtube.com/@code_wid_mapla")
		fmt.Println()
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Replace this binary with the latest GitHub release",
	RunE: func(cmd *cobra.Command, args []string) error {
		exePath, err := resolveExecutable()
		if err != nil {
			return err
		}
		// Checked before any network call: a system-wide install needs root,
		// and the user should hear that before waiting on a download.
		if err := ensureWritableInstall(exePath); err != nil {
			return err
		}

		fmt.Println("Checking for a newer version…")
		latest, err := latestReleaseTag()
		if err != nil {
			return fmt.Errorf("cannot check the latest version: %w", err)
		}
		if latest == Version {
			done("Already on the latest version (%s)", Version)
			return nil
		}
		note("Current: %s", Version)
		note("Latest:  %s", latest)
		fmt.Println()

		asset := fmt.Sprintf("envault-%s-%s", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Downloading %s…\n", asset)
		if err := replaceBinary(exePath, asset); err != nil {
			return err
		}

		done("Upgraded to %s", latest)
		note("Run 'envault version' to confirm.")
		return nil
	},
}

// replaceBinary downloads the release asset next to the running binary and
// swaps it in. Staging on the same filesystem is what makes the final step an
// atomic rename rather than a half-written executable.
func replaceBinary(exePath, asset string) error {
	url := "https://github.com/akhshyganesh/envault/releases/latest/download/" + asset

	resp, err := http.Get(url) //nolint:gosec // URL built from known GOOS/GOARCH
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d (no release binary for %s)", resp.StatusCode, asset)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, "envault-upgrade-*")
	if err != nil {
		return fmt.Errorf("cannot stage the download in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	_, err = io.Copy(tmp, resp.Body)
	tmp.Close()
	if err != nil {
		return fmt.Errorf("download interrupted: %w", err)
	}
	// 0755, unlike everything else envault writes: this one has to run.
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return fmt.Errorf("cannot replace the binary: %w (try sudo envault upgrade)", err)
	}
	return nil
}

// resolveExecutable follows symlinks, so upgrading replaces the binary itself
// rather than turning a link into a copy.
func resolveExecutable() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine the binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve the binary path: %w", err)
	}
	return resolved, nil
}

// ensureWritableInstall reports whether the binary can be replaced in place.
// The check is on the directory, not the file, because the download is staged
// beside it — and it probes rather than reading permission bits, which is the
// only way to be right about every filesystem.
func ensureWritableInstall(exePath string) error {
	dir := filepath.Dir(exePath)
	probe, err := os.CreateTemp(dir, ".envault-upgrade-check-*")
	if err != nil {
		return fmt.Errorf(
			"cannot upgrade: %s is not writable by the current user\n"+
				"  envault is installed at %s\n"+
				"  Re-run as: sudo envault upgrade",
			dir, exePath)
	}
	probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

func init() {
	rootCmd.AddCommand(versionCmd, upgradeCmd)
}
