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
	if _, err := Parse([]string{"--target", "nope"}, &buf); err == nil {
		t.Fatal("expected invalid IP error")
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
