package stages

import (
	"context"
	"installer/internal/commands"
	"installer/internal/runner"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// answers returns an Input that yields text, as piped stdin would.
func answers(t *testing.T, text string) *runner.Input {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	go func() { w.WriteString(text); w.Close() }()
	return runner.NewInput(r)
}

func newConfig(t *testing.T, hosts ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, h := range hosts {
		p := filepath.Join(dir, "hosts", "nixos", h)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "bootstrap.nix"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestChooseDisk(t *testing.T) {
	a := commands.Disk{Path: "/dev/vda", Size: 512e9}
	b := commands.Disk{Path: "/dev/nvme0n1", Size: 256e9}

	cases := []struct {
		name       string
		candidates []commands.Disk
		input      string
		want       string
		wantErr    string
	}{
		{"none keeps recorded", nil, "", "/dev/recorded", ""},
		{"single is taken without asking", []commands.Disk{a}, "", "/dev/vda", ""},
		{"several: operator picks", []commands.Disk{a, b}, "2\n", "/dev/nvme0n1", ""},
		{"several: bad answer", []commands.Disk{a, b}, "7\n", "", "--disk"},
		{"several: no answer", []commands.Disk{a, b}, "", "", "--disk"},
	}
	for _, c := range cases {
		s := &EnvironmentPreparationStage{In: answers(t, c.input)}
		got, err := s.chooseDisk(context.Background(), c.candidates, "/dev/recorded")
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err = %v, want containing %q", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q %v, want %q", c.name, got, err, c.want)
		}
	}
}
