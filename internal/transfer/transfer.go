package transfer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
)

// ErrVaultExists is returned by Import when a vault already exists and force is false.
var ErrVaultExists = errors.New("vault already exists")

// Export creates a zip archive of the entire vault directory.
// If outputPath is empty, a timestamped default filename is used in the current directory.
// Returns the absolute output path, the number of files archived, the archive size, and any error.
func Export(outputPath string) (absPath string, fileCount int, size int64, err error) {
	vaultDir := config.VaultDir()
	if _, statErr := os.Stat(vaultDir); os.IsNotExist(statErr) {
		return "", 0, 0, fmt.Errorf("no vault found at %s — run 'envault init' first", vaultDir)
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("envault-backup-%s.zip", time.Now().Format("20060102-150405"))
	}

	zipFile, createErr := os.Create(outputPath)
	if createErr != nil {
		return "", 0, 0, fmt.Errorf("cannot create output file: %w", createErr)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)

	skipFiles := map[string]bool{
		"envault.pid": true,
		"envault.log": true,
	}

	err = filepath.Walk(vaultDir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, relErr := filepath.Rel(vaultDir, p)
		if relErr != nil {
			return relErr
		}
		if skipFiles[relPath] || info.IsDir() {
			return nil
		}
		header, hErr := zip.FileInfoHeader(info)
		if hErr != nil {
			return hErr
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		writer, wErr := zipWriter.CreateHeader(header)
		if wErr != nil {
			return wErr
		}
		f, fErr := os.Open(p)
		if fErr != nil {
			return fErr
		}
		defer f.Close()

		_, copyErr := io.Copy(writer, f)
		fileCount++
		return copyErr
	})

	if closeErr := zipWriter.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(outputPath)
		return "", 0, 0, fmt.Errorf("export failed: %w", err)
	}

	absPath, _ = filepath.Abs(outputPath)
	info, _ := os.Stat(absPath)
	return absPath, fileCount, info.Size(), nil
}

// Import extracts a vault zip archive into the vault directory.
// Returns ErrVaultExists (wrapped) when the vault already exists and force is false.
// Returns the number of files extracted and any error.
func Import(zipPath string, force bool) (fileCount int, err error) {
	if _, statErr := os.Stat(zipPath); os.IsNotExist(statErr) {
		return 0, fmt.Errorf("file not found: %s", zipPath)
	}

	vaultDir := config.VaultDir()
	if info, statErr := os.Stat(vaultDir); statErr == nil && info.IsDir() && !force {
		return 0, fmt.Errorf("%w at %s", ErrVaultExists, vaultDir)
	}

	r, openErr := zip.OpenReader(zipPath)
	if openErr != nil {
		return 0, fmt.Errorf("cannot open zip: %w", openErr)
	}
	defer r.Close()

	if mkErr := os.MkdirAll(vaultDir, 0700); mkErr != nil {
		return 0, fmt.Errorf("cannot create vault directory: %w", mkErr)
	}

	for _, f := range r.File {
		target, pathErr := sanitizeExtractPath(vaultDir, f.Name)
		if pathErr != nil {
			return fileCount, pathErr
		}
		if f.FileInfo().IsDir() {
			if mkErr := os.MkdirAll(target, 0700); mkErr != nil {
				return fileCount, mkErr
			}
			continue
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), 0700); mkErr != nil {
			return fileCount, mkErr
		}
		rc, rcErr := f.Open()
		if rcErr != nil {
			return fileCount, fmt.Errorf("cannot read %s: %w", f.Name, rcErr)
		}
		outFile, outErr := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if outErr != nil {
			rc.Close()
			return fileCount, fmt.Errorf("cannot write %s: %w", target, outErr)
		}
		_, copyErr := io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if copyErr != nil {
			return fileCount, fmt.Errorf("cannot extract %s: %w", f.Name, copyErr)
		}
		fileCount++
	}

	return fileCount, nil
}

// sanitizeExtractPath prevents zip-slip attacks by ensuring the extracted
// path stays within the target directory.
func sanitizeExtractPath(baseDir, zipEntry string) (string, error) {
	target := filepath.Join(baseDir, filepath.FromSlash(zipEntry))
	cleanTarget := filepath.Clean(target) + string(os.PathSeparator)
	cleanBase := filepath.Clean(baseDir) + string(os.PathSeparator)
	if !strings.HasPrefix(cleanTarget, cleanBase) {
		return "", fmt.Errorf("invalid zip entry path: %s", zipEntry)
	}
	return target, nil
}
