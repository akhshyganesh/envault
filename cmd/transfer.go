package cmd

import (
	"errors"
	"fmt"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/transfer"
	"github.com/spf13/cobra"
)

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export all envault data as a zip archive for backup or migration",
	Long: `Creates a portable zip archive containing all backed-up .env files,
version history, and configuration. Use this before formatting your system
or to transfer backups to another machine.

Restore with 'envault import <file>'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		absOutput, fileCount, size, err := transfer.Export(exportOutput)
		if err != nil {
			return err
		}
		fmt.Printf("✓ Exported %d files to %s (%s)\n", fileCount, absOutput, format.HumanSize(size))
		fmt.Println("  Transfer this file to your new system and run 'envault import <file>'")
		return nil
	},
}

var importForce bool

var importCmd = &cobra.Command{
	Use:   "import <zipfile>",
	Short: "Import envault data from a previously exported zip archive",
	Long: `Restores all backed-up .env files, version history, and configuration
from a zip archive created by 'envault export'.

Use this after a fresh OS install to restore all your backups.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileCount, err := transfer.Import(args[0], importForce)
		if err != nil {
			if errors.Is(err, transfer.ErrVaultExists) {
				return fmt.Errorf("%v\nUse --force to overwrite existing data", err)
			}
			return err
		}
		fmt.Printf("✓ Imported %d files\n", fileCount)
		fmt.Println("  Run 'envault list' to see your restored files")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output zip file path (default: envault-backup-<timestamp>.zip)")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Overwrite existing vault data")
}
