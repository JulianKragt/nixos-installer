package stages

import (
	"context"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDialTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := dialTCP(context.Background(), addr); err != nil {
		t.Errorf("listening port: %v", err)
	}
	ln.Close()
	if err := dialTCP(context.Background(), addr); err == nil {
		t.Error("closed port: want an error")
	}
}

func TestCheckConfig(t *testing.T) {
	dir := t.TempDir()
	if err := checkConfig(dir); err == nil {
		t.Error("want an error without flake.nix")
	}
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkConfig(dir); err != nil {
		t.Error(err)
	}
}

func TestResolveDiskPrefersTheFlag(t *testing.T) {
	st, err := pipeline.LoadState(t.TempDir(), "h")
	if err != nil {
		t.Fatal(err)
	}
	// Remote is this machine: asking it for disks would fail the test on
	// machines without lsblk, so the flag must short-circuit.
	env := &pipeline.Env{Inputs: pipeline.Inputs{Disk: "/dev/flag"}, State: st}
	got, err := resolveDisk(context.Background(), env)
	if err != nil || got != "/dev/flag" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestParseLsblk(t *testing.T) {
	out := `loop0 1000 loop 0
sda 31457280000 disk 1
nvme0n1 512110190592 disk 0
sr0 1073741312 rom 0
sdb 1000204886016 disk 0
sdc 0 disk 0
`
	got := parseLsblk(out)
	if len(got) != 2 || got[0].Path != "/dev/nvme0n1" || got[0].Size != 512110190592 || got[1].Path != "/dev/sdb" {
		t.Fatalf("got %+v", got)
	}
	if s := got[0].String(); s != "/dev/nvme0n1 (512 GB)" {
		t.Fatalf("String() = %q", s)
	}
}

func TestChooseDisk(t *testing.T) {
	a := Disk{Path: "/dev/vda", Size: 512e9}
	b := Disk{Path: "/dev/nvme0n1", Size: 256e9}

	cases := []struct {
		name       string
		candidates []Disk
		recorded   string
		input      string
		want       string
		wantErr    string
	}{
		{"none despite recorded", nil, "/dev/recorded", "", "", "--disk"},
		{"none and nothing recorded", nil, "", "", "", "--disk"},
		{"recorded wins without asking", []Disk{a, b}, "/dev/nvme0n1", "", "/dev/nvme0n1", ""},
		{"single is taken without asking", []Disk{a}, "/dev/recorded", "", "/dev/vda", ""},
		{"several: operator picks", []Disk{a, b}, "/dev/recorded", "2\n", "/dev/nvme0n1", ""},
		{"several: bad answer", []Disk{a, b}, "/dev/recorded", "7\n", "", "--disk"},
		{"several: no answer", []Disk{a, b}, "/dev/recorded", "", "", "--disk"},
	}
	for _, c := range cases {
		ui := runner.New(runner.Options{In: strings.NewReader(c.input)})
		got, err := chooseDisk(runner.NewContext(context.Background(), ui), c.candidates, c.recorded)
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
