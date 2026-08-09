package format

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:               "0B",
		512:             "512B",
		1024:            "1.0KB",
		1536:            "1.5KB",
		1024 * 1024:     "1.0MB",
		3 * 1024 * 1024: "3.0MB",
	}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()
	cases := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := TimeAgo(c.at); got != c.want {
			t.Errorf("TimeAgo(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

// homeDir is cached with sync.OnceValue, so these exercise the real home rather
// than a t.Setenv one — which is the behaviour that actually ships.
func TestShortenPathAbbreviatesHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}

	inHome := filepath.Join(home, "projects", ".env")
	if got, want := ShortenPath(inHome), filepath.Join("~", "projects", ".env"); got != want {
		t.Errorf("ShortenPath(%q) = %q, want %q", inHome, got, want)
	}
	if got := ShortenPath("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("ShortenPath(/etc/hosts) = %q, want it unchanged", got)
	}
}

func TestExpandPathRoundTripsWithShorten(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}

	got, err := ExpandPath("~/projects/.env")
	if err != nil {
		t.Fatalf("ExpandPath: %v", err)
	}
	if want := filepath.Join(home, "projects", ".env"); got != want {
		t.Fatalf("ExpandPath(~/projects/.env) = %q, want %q", got, want)
	}
	if back := ShortenPath(got); back != "~/projects/.env" {
		t.Fatalf("ShortenPath did not undo ExpandPath: %q", back)
	}
}

func TestExpandPathMakesRelativePathsAbsolute(t *testing.T) {
	got, err := ExpandPath("relative/.env")
	if err != nil {
		t.Fatalf("ExpandPath: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("ExpandPath(relative/.env) = %q, want an absolute path", got)
	}
}
