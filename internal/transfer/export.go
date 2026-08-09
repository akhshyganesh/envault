package transfer

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
)

// ExportResult describes the archive that was written.
type ExportResult struct {
	Path  string
	Files int
	Size  int64
}

// DefaultExportName is the timestamped filename used when none is given.
func DefaultExportName() string {
	return fmt.Sprintf("envault-backup-%s.zip", time.Now().Format("20060102-150405"))
}

// Export writes the whole vault to a zip archive. A failed export removes its
// own half-written file rather than leaving a corrupt archive behind.
func Export(outputPath string) (*ExportResult, error) {
	vaultDir := config.VaultDir()
	if _, err := os.Stat(vaultDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no vault at %s — run 'envault scan' first", vaultDir)
	}
	if outputPath == "" {
		outputPath = DefaultExportName()
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", outputPath, err)
	}
	defer out.Close()

	count, err := writeVaultZip(out, vaultDir)
	if err != nil {
		_ = os.Remove(outputPath)
		return nil, fmt.Errorf("export failed: %w", err)
	}

	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	return &ExportResult{Path: abs, Files: count, Size: info.Size()}, nil
}

func writeVaultZip(out io.Writer, vaultDir string) (int, error) {
	zw := zip.NewWriter(out)
	count := 0

	err := filepath.Walk(vaultDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(vaultDir, p)
		if err != nil {
			return err
		}
		if info.IsDir() || runtimeFiles[rel] {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate

		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(w, f); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		_ = zw.Close()
		return 0, err
	}
	return count, zw.Close()
}
