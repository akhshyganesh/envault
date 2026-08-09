package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/transfer"
)

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the whole vault as a zip archive",
	Long: `Writes every backed-up file, its history, and your config into one zip.

Use it before reinstalling an OS or moving to another machine, then restore
with 'envault import <file>'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := transfer.Export(exportOutput)
		if err != nil {
			return err
		}
		done("Exported %d %s to %s (%s)",
			res.Files, plural(res.Files, "file", "files"), res.Path, format.HumanSize(res.Size))
		note("Restore it with 'envault import %s'", res.Path)
		return nil
	},
}

var importForce bool

var importCmd = &cobra.Command{
	Use:   "import <file.zip>",
	Short: "Restore a vault from an exported zip archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		count, err := transfer.Import(args[0], importForce)
		if err != nil {
			if errors.Is(err, transfer.ErrVaultExists) {
				return fmt.Errorf("%w — pass --force to replace it", err)
			}
			return err
		}
		done("Imported %d %s", count, plural(count, "file", "files"))
		note("Run 'envault list' to see them.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd, importCmd)
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "",
		"Where to write the zip (default: envault-backup-<timestamp>.zip)")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Replace an existing vault")
}
