package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/format"
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
		vaultDir := config.VaultDir()
		if _, err := os.Stat(vaultDir); os.IsNotExist(err) {
			return fmt.Errorf("no vault found at %s — run 'envault init' first", vaultDir)
		}

		output := exportOutput
		if output == "" {
			output = fmt.Sprintf("envault-backup-%s.zip", time.Now().Format("20060102-150405"))
		}

		zipFile, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("cannot create output file: %w", err)
		}
		defer zipFile.Close()

		zipWriter := zip.NewWriter(zipFile)

		skipFiles := map[string]bool{
			"envault.pid": true,
			"envault.log": true,
		}

		var fileCount int
		err = filepath.Walk(vaultDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(vaultDir, path)
			if err != nil {
				return err
			}

			if skipFiles[relPath] || info.IsDir() {
				return nil
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)
			header.Method = zip.Deflate

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			fileCount++
			return err
		})

		if closeErr := zipWriter.Close(); closeErr != nil && err == nil {
			err = closeErr
		}

		if err != nil {
			os.Remove(output)
			return fmt.Errorf("export failed: %w", err)
		}

		absOutput, _ := filepath.Abs(output)
		info, _ := os.Stat(absOutput)
		fmt.Printf("✓ Exported %d files to %s (%s)\n", fileCount, absOutput, format.HumanSize(info.Size()))
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
		zipPath := args[0]

		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", zipPath)
		}

		vaultDir := config.VaultDir()

		if info, err := os.Stat(vaultDir); err == nil && info.IsDir() && !importForce {
			return fmt.Errorf("vault already exists at %s\nUse --force to overwrite existing data", vaultDir)
		}

		r, err := zip.OpenReader(zipPath)
		if err != nil {
			return fmt.Errorf("cannot open zip file: %w", err)
		}
		defer r.Close()

		if err := os.MkdirAll(vaultDir, 0700); err != nil {
			return fmt.Errorf("cannot create vault directory: %w", err)
		}

		var fileCount int
		for _, f := range r.File {
			target, err := sanitizeExtractPath(vaultDir, f.Name)
			if err != nil {
				return err
			}

			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(target, 0700); err != nil {
					return err
				}
				continue
			}

			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}

			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("cannot read zip entry %s: %w", f.Name, err)
			}

			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				rc.Close()
				return fmt.Errorf("cannot write %s: %w", target, err)
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return fmt.Errorf("cannot extract %s: %w", f.Name, err)
			}
			fileCount++
		}

		fmt.Printf("✓ Imported %d files to %s\n", fileCount, vaultDir)
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
