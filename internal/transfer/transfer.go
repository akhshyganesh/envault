// Package transfer moves a whole vault in and out of a zip archive, for
// migrating between machines — and reads such an archive in place, without
// importing it.
package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runtimeFiles belong to a live daemon, not to the backup. Carrying a stale
// PID into another machine's vault would make it think a daemon is running.
var runtimeFiles = map[string]bool{
	"envault.pid": true,
	"envault.log": true,
}

// safeExtractPath joins a zip entry onto a directory, refusing entries that
// would escape it. Without this check an archive containing "../../.ssh/id_rsa"
// would overwrite files far outside the vault (a zip-slip).
func safeExtractPath(baseDir, entry string) (string, error) {
	target := filepath.Join(baseDir, filepath.FromSlash(entry))
	sep := string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target)+sep, filepath.Clean(baseDir)+sep) {
		return "", fmt.Errorf("refusing zip entry that escapes the vault: %s", entry)
	}
	return target, nil
}
