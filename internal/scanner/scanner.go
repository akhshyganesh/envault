// Package scanner walks directory trees looking for env files and hands each
// one to the store.
package scanner

import (
	"os"
	"path/filepath"

	"github.com/akhshyganesh/envault/internal/store"
)

// Result summarises one scan. Errors are collected rather than returned so a
// single unreadable directory does not abandon the rest of the walk.
type Result struct {
	Found   []string
	Backed  int // new snapshots created
	Skipped int // files whose content was unchanged
	Errors  []error
}

// ScanDirectory walks one tree, backing up every env file it finds.
func ScanDirectory(dir string, s *store.Store) (*Result, error) {
	res := &Result{}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Usually a permission denial. Skip the subtree when the failure is
			// on a directory; skipping on a file would abandon its siblings too.
			res.Errors = append(res.Errors, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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

		res.Found = append(res.Found, path)
		_, isNew, err := s.SaveSnapshot(path, "auto")
		if err != nil {
			res.Errors = append(res.Errors, err)
			return nil
		}
		if isNew {
			res.Backed++
		} else {
			res.Skipped++
		}
		return nil
	})

	return res, err
}

// ScanDirectories walks several trees and merges their results.
func ScanDirectories(dirs []string, s *store.Store) (*Result, error) {
	combined := &Result{}
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
