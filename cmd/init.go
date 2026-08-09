package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create the vault directory and a default config",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := os.Stat(config.VaultDir())
		firstRun := os.IsNotExist(err)

		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return err
		}

		if !firstRun {
			done("Vault re-initialised at %s", config.VaultDir())
			note("Config:   %s", config.ConfigPath())
			note("Watching: %s", strings.Join(cfg.WatchDirs, ", "))
			return nil
		}

		printWelcome(cfg)
		return nil
	},
}

func printWelcome(cfg *config.Config) {
	fmt.Println()
	fmt.Println("  ┌──────────────────────────────────────────────────┐")
	fmt.Println("  │  Welcome to envault                              │")
	fmt.Println("  │  Your .env files, safely vaulted.                │")
	fmt.Println("  └──────────────────────────────────────────────────┘")
	fmt.Println()
	note("%s Vault created at %s", mark, config.VaultDir())
	note("%s Config:         %s", mark, config.ConfigPath())
	note("%s Watching:       %s", mark, strings.Join(cfg.WatchDirs, ", "))
	fmt.Println()
	fmt.Println("  ── Next ──────────────────────────────────────────")
	note("envault scan    take your first backup")
	note("envault start   run the daemon in the background")
	fmt.Println()
	fmt.Println("  ── Project ───────────────────────────────────────")
	note("Source:    https://github.com/akhshyganesh/envault")
	note("Tutorials: https://www.youtube.com/@code_wid_mapla")
	fmt.Println()
}

func init() {
	rootCmd.AddCommand(initCmd)
}
