package stages

import (
	"context"
	"installer/internal/runner"
	"strings"
	"testing"
)

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
		{"none keeps recorded", nil, "/dev/recorded", "", "/dev/recorded", ""},
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
