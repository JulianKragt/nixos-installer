package main

import (
	"bytes"
	"context"
	"installer/internal/cli"
	"installer/internal/runner"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInputsLoadsStateAndTarget(t *testing.T) {
	cfg := t.TempDir()
	host := filepath.Join(cfg, "hosts", "nixos", "atlas")
	if err := os.MkdirAll(host, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "bootstrap.nix"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	w.Close()

	var out bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &out}))
	opts := cli.Options{ConfigDir: cfg, Host: "atlas", TargetIP: "192.168.1.100"}
	st, err := resolveInputs(ctx, &opts, runner.NewInput(r), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st.HostName != "atlas" || st.Target != "192.168.1.100" {
		t.Errorf("state: %+v", st)
	}
}
