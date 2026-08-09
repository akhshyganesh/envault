package transfer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/akhshyganesh/envault/internal/config"
)

// ErrVaultExists means importing would overwrite a vault that is already
// there. Callers should offer --force rather than silently replacing backups.
var ErrVaultExists = errors.New("vault already exists")

// Import extracts an archive into the vault directory, returning how many
// files it wrote.
func Import(zipPath string, force bool) (int, error) {
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("file not found: %s", zipPath)
	}

	vaultDir := config.VaultDir()
	if info, err := os.Stat(vaultDir); err == nil && info.IsDir() && !force {
		return 0, fmt.Errorf("%w at %s", ErrVaultExists, vaultDir)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("cannot open %s: %w", zipPath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return 0, fmt.Errorf("creating vault directory: %w", err)
	}

	count := 0
	for _, f := range r.File {
		target, err := safeExtractPath(vaultDir, f.Name)
		if err != nil {
			return count, err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return count, err
			}
			continue
		}
		if err := extractFile(f, target); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func extractFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", target, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("cannot extract %s: %w", f.Name, err)
	}
	return nil
}
