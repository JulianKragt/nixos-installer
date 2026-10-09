package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func asker(input string) Asker {
	in := bufio.NewReader(strings.NewReader(input))
	return func(string) (string, error) {
		line, err := in.ReadString('\n')
		return strings.TrimSpace(line), err
	}
}

func TestParse(t *testing.T) {
	var buf bytes.Buffer
	o, err := Parse([]string{"--target", "192.168.1.100", "--host", "atlas"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if o.TargetIP != "192.168.1.100" || o.Host != "atlas" || o.ConfigDir != "./" || o.Disk != "" {
		t.Fatalf("got %+v", o)
	}
	empty, err := Parse(nil, &buf)
	if err != nil || empty.TargetIP != "" {
		t.Fatalf("empty target should parse: %+v %v", empty, err)
	}
	if err := empty.CompleteTarget(asker("nope\n10.0.0.5\n")); err != nil || empty.TargetIP != "10.0.0.5" {
		t.Fatalf("complete: %+v %v", empty, err)
	}
	var none Options
	if err := none.CompleteTarget(asker("")); err == nil {
		t.Fatal("expected error on EOF")
	}
	for _, bad := range []string{"nope", "127.0.0.1", "::1", "0.0.0.0", "224.0.0.1"} {
		if _, err := Parse([]string{"--target", bad}, &buf); err == nil {
			t.Fatalf("expected invalid target error for %q", bad)
		}
	}
	for _, good := range []string{"2001:db8::5", "fe80::1%eth0"} {
		if _, err := Parse([]string{"--target", good}, &buf); err != nil {
			t.Fatalf("IPv6 target %q: %v", good, err)
		}
	}
	if _, err := Parse([]string{"--target", "fe80::1"}, &buf); err == nil || !strings.Contains(err.Error(), "zone") {
		t.Fatalf("link-local without zone should ask for a zone, got %v", err)
	}
}

func TestListAndPickHost(t *testing.T) {
	dir := t.TempDir()
	for _, h := range []string{"broadway", "atlas"} {
		p := filepath.Join(dir, "hosts", "nixos", h)
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "bootstrap.nix"), nil, 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "hosts", "nixos", "nobootstrap"), 0o755)

	hosts, err := ListHosts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hosts, []string{"atlas", "broadway"}) {
		t.Fatalf("got %v", hosts)
	}

	got, err := PickHost(asker("2\n"), hosts, "")
	if err != nil || got != "broadway" {
		t.Fatalf("pick: %q %v", got, err)
	}
	if got, err := PickHost(nil, hosts, "atlas"); err != nil || got != "atlas" {
		t.Fatalf("want: %q %v", got, err)
	}
	if _, err := PickHost(nil, hosts, "x"); err == nil {
		t.Fatal("expected unknown host error")
	}
}
