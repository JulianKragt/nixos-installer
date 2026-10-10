package stages

import (
	"context"
	"installer/internal/pipeline"
	"net"
	"os"
	"path/filepath"
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
