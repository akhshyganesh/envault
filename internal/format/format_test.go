package format

import (
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
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := TimeAgo(c.t); got != c.want {
			t.Errorf("TimeAgo(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	// homeDir() is cached via sync.OnceValue, so this only holds in a fresh
	// process; assert the non-home passthrough which is environment-independent.
	if got := ShortenPath("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("ShortenPath(/etc/hosts) = %q, want unchanged", got)
	}
}
