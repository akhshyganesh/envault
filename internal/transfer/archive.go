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

// Archive is an export zip opened for reading in place. Nothing it does
// touches the local vault — that is the whole point of 'envault peek'.
type Archive struct {
	Path string

	r     *zip.ReadCloser
	files []store.FileHistory
	blobs map[string]*zip.File // snapshot ID -> zip entry
	cfg   *zip.File
}

// OpenArchive reads an export zip's index. The caller must Close it.
//
// A zip with neither an index/ nor a config.json is rejected: that is the
// cheapest way to tell an envault export from any other zip the user typed.
func OpenArchive(zipPath string) (*Archive, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", zipPath, err)
	}

	a := &Archive{Path: zipPath, r: r, blobs: make(map[string]*zip.File)}
	for _, f := range r.File {
		dir, base := path.Split(path.Clean(f.Name))
		switch {
		case dir == "blobs/" && base != "":
			a.blobs[base] = f
		case dir == "index/" && strings.HasSuffix(base, ".json"):
			h, err := readHistory(f)
			if err != nil {
				r.Close()
				return nil, err
			}
			a.files = append(a.files, *h)
		case f.Name == "config.json":
			a.cfg = f
		}
	}

	if a.cfg == nil && len(a.files) == 0 {
		r.Close()
		return nil, fmt.Errorf("%s is not an envault export (no index/ or config.json)", zipPath)
	}

	sort.Slice(a.files, func(i, j int) bool { return a.files[i].FilePath < a.files[j].FilePath })
	return a, nil
}

// Close releases the zip reader.
func (a *Archive) Close() error { return a.r.Close() }

// Files lists the archive's tracked files, sorted by path so the numbering
// matches what was printed.
func (a *Archive) Files() []store.FileHistory { return a.files }

// Config returns the settings captured in the archive.
func (a *Archive) Config() (*config.Config, error) {
	if a.cfg == nil {
		return nil, fmt.Errorf("archive has no config.json")
	}
	data, err := readAll(a.cfg)
	if err != nil {
		return nil, err
	}
	cfg := config.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("cannot parse the archive's config.json: %w", err)
	}
	return cfg, nil
}

// Resolve accepts a 1-based index from Files, a full path, or an unambiguous
// path suffix such as "myproject/.env".
func (a *Archive) Resolve(arg string) (*store.FileHistory, error) {
	if len(a.files) == 0 {
		return nil, fmt.Errorf("archive contains no tracked files")
	}

	if idx, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil {
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

// Content returns a snapshot's bytes from inside the archive.
func (a *Archive) Content(snapshotID string) ([]byte, error) {
	f, ok := a.blobs[snapshotID]
	if !ok {
		return nil, fmt.Errorf("blob %s is missing from the archive", snapshotID)
	}
	return readAll(f)
}

func readHistory(f *zip.File) (*store.FileHistory, error) {
	data, err := readAll(f)
	if err != nil {
		return nil, err
	}
	var h store.FileHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", f.Name, err)
	}
	return &h, nil
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", f.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", f.Name, err)
	}
	return data, nil
}
