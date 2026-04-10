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

		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			return fmt.Errorf("cannot resolve binary path: %w", err)
		}

		tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "envault-upgrade-*")
		if err != nil {
			return fmt.Errorf("cannot create temp file: %w (try running with sudo)", err)
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

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
}
