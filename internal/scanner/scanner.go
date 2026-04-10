package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/akhshyganesh/envault/internal/store"
)

// skipDirs is the set of directory names that the scanner never descends into.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".svn":         true,
	".hg":          true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".tox":         true,
	"dist":         true,
	"build":        true,
	".envault":     true,
}

func shouldSkipDir(name string) bool {
	return skipDirs[name]
}

// isEnvFile checks if a filename matches env file patterns.
// Matches:
//   - .env
//   - .env.* (.env.local, .env.production, .env.sample, .env.development, etc.)
//   - *.env  (production.env, staging.env, app.env, etc.)
func isEnvFile(name string) bool {
	base := filepath.Base(name)

	// Exact match: .env
	if base == ".env" {
		return true
	}

	// Prefix match: .env.* (.env.local, .env.sample, .env.production, etc.)
	if strings.HasPrefix(base, ".env.") {
		return true
	}

	// Suffix match: *.env (production.env, staging.env, etc.)
	if strings.HasSuffix(base, ".env") && base != ".env" {
		return true
	}

	return false
}

// ScanResult holds the result of scanning a directory tree.
type ScanResult struct {
	Found   []string // env files found
	Backed  int      // new snapshots created
	Skipped int      // unchanged files (already latest)
	Errors  []error  // any errors encountered
}

// ScanDirectory walks a directory tree, finds all .env files, and backs them up.
func ScanDirectory(dir string, s *store.Store) (*ScanResult, error) {
	result := &ScanResult{}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Permission denied, etc. — skip but record
			result.Errors = append(result.Errors, err)
			return filepath.SkipDir
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isEnvFile(d.Name()) {
			return nil
		}

		result.Found = append(result.Found, path)

		_, isNew, err := s.SaveSnapshot(path, "auto")
		if err != nil {
			result.Errors = append(result.Errors, err)
			return nil
		}
		if isNew {
			result.Backed++
		} else {
			result.Skipped++
		}

		return nil
	})

	return result, err
}

// ScanDirectories scans multiple directories.
func ScanDirectories(dirs []string, s *store.Store) (*ScanResult, error) {
	combined := &ScanResult{}

	for _, dir := range dirs {
		res, err := ScanDirectory(dir, s)
		if err != nil {
			combined.Errors = append(combined.Errors, err)
			continue
		}
		combined.Found = append(combined.Found, res.Found...)
		combined.Backed += res.Backed
		combined.Skipped += res.Skipped
		combined.Errors = append(combined.Errors, res.Errors...)
	}

	return combined, nil
}
