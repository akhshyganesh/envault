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
	Short: "Print envault version and build info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🔒 envault — your .env files, safely vaulted")
		fmt.Println()
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Built:      %s\n", BuildDate)
		fmt.Println()
		fmt.Println("  Created by: Akhshy (Code_Wid_Mapla)")
		fmt.Println("  GitHub:     https://github.com/akhshyganesh/envault")
		fmt.Println("  YouTube:    https://www.youtube.com/@code_wid_mapla")
		fmt.Println()
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade envault to the latest release from GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		exePath, err := resolveExecutable()
		if err != nil {
			return err
		}
		// Check this before touching the network: a system-wide install needs
		// root, and the user should hear that before a download starts.
		if err := ensureWritableInstall(exePath); err != nil {
			return err
		}

		fmt.Println("Checking for latest version...")

		latestVersion, err := getLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to check latest version: %w", err)
		}

		if latestVersion == Version {
			fmt.Printf("✓ Already on the latest version (%s)\n", Version)
			return nil
		}

		fmt.Printf("  Current: %s\n", Version)
		fmt.Printf("  Latest:  %s\n", latestVersion)
		fmt.Println()

		assetName := fmt.Sprintf("envault-%s-%s", runtime.GOOS, runtime.GOARCH)
		downloadURL := fmt.Sprintf("https://github.com/akhshyganesh/envault/releases/latest/download/%s", assetName)

		fmt.Printf("Downloading %s...\n", assetName)

		resp, err := http.Get(downloadURL) //nolint:gosec // URL constructed from known-safe GOOS/GOARCH values
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed: HTTP %d (no release binary for %s)", resp.StatusCode, assetName)
		}

		tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "envault-upgrade-*")
		if err != nil {
			return fmt.Errorf("cannot stage the download in %s: %w", filepath.Dir(exePath), err)
		}
		tmpPath := tmpFile.Name()

		_, err = io.Copy(tmpFile, resp.Body)
		tmpFile.Close()
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("download interrupted: %w", err)
		}

		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot set permissions: %w", err)
		}

		if err := os.Rename(tmpPath, exePath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot replace binary: %w (try running with sudo)", err)
		}

		fmt.Printf("\n✓ Upgraded envault to %s\n", latestVersion)
		fmt.Println("  Run 'envault version' to verify.")
		return nil
	},
}

// resolveExecutable returns the real path of the running binary, following
// symlinks so the upgrade replaces the binary itself and not a link to it.
func resolveExecutable() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve binary path: %w", err)
	}
	return resolved, nil
}

// ensureWritableInstall reports whether the running binary can be replaced in
// place. The upgrade stages its download next to the binary so the final swap
// is an atomic rename on the same filesystem, which means the install
// directory — not just the binary — has to be writable.
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
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
}
