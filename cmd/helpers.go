package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/akhshyganesh/envault/internal/store"
)

// resolveFileArg accepts either an absolute file path or a numeric index
// from 'envault list' and returns the absolute file path.
func resolveFileArg(arg string, s *store.Store) (string, error) {
	if idx, err := strconv.Atoi(arg); err == nil {
		files, err := s.ListTrackedFiles()
		if err != nil {
			return "", err
		}
		if idx < 1 || idx > len(files) {
			return "", fmt.Errorf("index %d out of range (1-%d). Run 'envault list' to see indices", idx, len(files))
		}
		return files[idx-1].FilePath, nil
	}
	return filepath.Abs(arg)
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

// getLatestVersion fetches the latest release tag from GitHub.
func getLatestVersion() (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("https://api.github.com/repos/akhshyganesh/envault/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}
