package commands

import "testing"

func TestParseLsblk(t *testing.T) {
	out := `loop0 1000 loop 0
sda 31457280000 disk 1
nvme0n1 512110190592 disk 0
sr0 1073741312 rom 0
`
	got := parseLsblk(out)
	if len(got) != 1 || got[0].path != "/dev/nvme0n1" || got[0].size != 512110190592 {
		t.Fatalf("got %+v", got)
	}
}
