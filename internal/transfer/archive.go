// ABOUTME: Read-only reader for an 'envault export' zip archive.
// ABOUTME: Lets callers browse and extract vault contents without importing them.
package transfer

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/store"
)

// Archive is an open export zip, browsed in place. Nothing it does reads or
// writes the local vault directory.
type Archive struct {
	Path  string
	r     *zip.ReadCloser
	files []store.FileHistory
	blobs map[string]*zip.File // snapshot ID -> zip entry
	cfg   *zip.File
}

// OpenArchive opens an export zip and reads its history index. The caller must
// Close it. It fails if the zip contains no vault index, which is the cheapest
// way to tell an envault export from any other zip.
func OpenArchive(zipPath string) (*Archive, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", zipPath, err)
	}

	a := &Archive{Path: zipPath, r: r, blobs: make(map[string]*zip.File)}
	for _, f := range r.File {
		name := path.Clean(f.Name)
		dir, base := path.Split(name)
		switch {
		case dir == "blobs/" && base != "":
			a.blobs[base] = f
		case dir == "index/" && strings.HasSuffix(base, ".json"):
			h, hErr := readHistory(f)
			if hErr != nil {
				r.Close()
				return nil, hErr
			}
			a.files = append(a.files, *h)
		case name == "config.json":
			a.cfg = f
		}
	}

	if a.cfg == nil && len(a.files) == 0 {
		r.Close()
		return nil, fmt.Errorf("%s does not look like an envault export (no index/ or config.json)", zipPath)
	}

	sort.Slice(a.files, func(i, j int) bool {
		return a.files[i].FilePath < a.files[j].FilePath
	})
	return a, nil
}

// Close releases the underlying zip reader.
func (a *Archive) Close() error {
	return a.r.Close()
}

// Files returns the tracked files in the archive, sorted by path.
func (a *Archive) Files() []store.FileHistory {
	return a.files
}

// Config returns the configuration captured in the archive.
func (a *Archive) Config() (*config.Config, error) {
	if a.cfg == nil {
		return nil, fmt.Errorf("archive contains no config.json")
	}
	rc, err := a.cfg.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	cfg := config.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("cannot parse archive config.json: %w", err)
	}
	return cfg, nil
}

// Resolve accepts a 1-based index from Files(), a full file path, or an
// unambiguous path suffix, and returns the matching history.
func (a *Archive) Resolve(arg string) (*store.FileHistory, error) {
	if len(a.files) == 0 {
		return nil, fmt.Errorf("archive contains no tracked files")
	}

	if idx, err := strconv.Atoi(arg); err == nil {
		if idx < 1 || idx > len(a.files) {
			return nil, fmt.Errorf("index %d out of range (1-%d)", idx, len(a.files))
		}
		return &a.files[idx-1], nil
	}

	var matches []*store.FileHistory
	for i := range a.files {
		if a.files[i].FilePath == arg {
			return &a.files[i], nil
		}
		if strings.HasSuffix(a.files[i].FilePath, "/"+strings.TrimPrefix(arg, "/")) {
			matches = append(matches, &a.files[i])
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("%s is not in this archive", arg)
	default:
		paths := make([]string, len(matches))
		for i, m := range matches {
			paths[i] = m.FilePath
		}
		return nil, fmt.Errorf("%s matches %d files — use the # or a full path:\n  %s",
			arg, len(matches), strings.Join(paths, "\n  "))
	}
}

// Content returns the bytes of a snapshot stored in the archive.
func (a *Archive) Content(snapshotID string) ([]byte, error) {
	f, ok := a.blobs[snapshotID]
	if !ok {
		return nil, fmt.Errorf("blob %s is missing from the archive", snapshotID)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// readHistory decodes one index/*.json entry.
func readHistory(f *zip.File) (*store.FileHistory, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", f.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", f.Name, err)
	}
	var h store.FileHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", f.Name, err)
	}
	return &h, nil
}
