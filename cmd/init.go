package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize envault (creates config and vault directory)",
	RunE: func(cmd *cobra.Command, args []string) error {
		firstTime := false
		if _, err := os.Stat(config.VaultDir()); os.IsNotExist(err) {
			firstTime = true
		}

		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return err
		}

		if firstTime {
			fmt.Println()
			fmt.Println("  ┌──────────────────────────────────────────────────┐")
			fmt.Println("  │  Welcome to envault                              │")
			fmt.Println("  │  Your .env files, safely vaulted.                │")
			fmt.Println("  └──────────────────────────────────────────────────┘")
			fmt.Println()
			fmt.Printf("  ✓ Vault created at %s\n", config.VaultDir())
			fmt.Printf("  ✓ Config: %s\n", config.ConfigPath())
			fmt.Printf("  ✓ Watching: %s\n", strings.Join(cfg.WatchDirs, ", "))
			fmt.Println()
			fmt.Println("  ── Get Started ──────────────────────────────────")
			fmt.Println("  Run 'envault scan' to do your first backup")
			fmt.Println("  Run 'envault start' to launch the background daemon")
			fmt.Println()
			fmt.Println("  ── Support the Project ──────────────────────────")
			fmt.Println("  Star us on GitHub: https://github.com/akhshyganesh/envault")
			fmt.Println("  Watch tutorials:   https://www.youtube.com/@code_wid_mapla")
			fmt.Println()
			fmt.Println("  Made by Akhshy (Code_Wid_Mapla)")
			fmt.Println()
		} else {
			fmt.Printf("✓ envault re-initialized at %s\n", config.VaultDir())
			fmt.Printf("  Config: %s\n", config.ConfigPath())
			fmt.Printf("  Watching: %s\n", strings.Join(cfg.WatchDirs, ", "))
			fmt.Println("\nRun 'envault scan' to do your first backup, or 'envault start' to run the daemon.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
