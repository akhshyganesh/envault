package scanner

import (
	"path/filepath"
	"strings"
)

// skipDirs are never descended into. They are either enormous and full of
// vendored env files that are not the user's (node_modules, vendor), or they
// are envault's own storage.
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

// isEnvFile recognises the three shapes env files come in:
//
//	.env            the plain one
//	.env.*          .env.local, .env.production, .env.sample …
//	*.env           production.env, staging.env …
//
// Note that "env" and ".environment" match none of these on purpose.
func isEnvFile(name string) bool {
	base := filepath.Base(name)
	switch {
	case base == ".env":
		return true
	case strings.HasPrefix(base, ".env."):
		return true
	case strings.HasSuffix(base, ".env"):
		return true
	default:
		return false
	}
}
