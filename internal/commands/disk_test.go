package commands

import (
	"context"
	"installer/internal/command"
	"installer/internal/command/executor"
	"testing"
)

func TestParseLsblk(t *testing.T) {
	out := `loop0 1000 loop 0
sda 31457280000 disk 1
nvme0n1 512110190592 disk 0
sr0 1073741312 rom 0
`
	got := parseLsblk(out)
	if len(got) != 1 || got[0].Path != "/dev/nvme0n1" || got[0].Size != 512110190592 {
		t.Fatalf("got %+v", got)
	}
}

type lsblkExec string

func (o lsblkExec) Run(context.Context, command.Command, executor.ExecOptions) (command.Result, error) {
	return command.Result{Output: string(o)}, nil
}

func TestListDisks(t *testing.T) {
	out := lsblkExec("nvme0n1 512110190592 disk 0\nsda 31457280000 disk 1\nsdb 1000204886016 disk 0\n")
	got, err := ListDisks(context.Background(), out)
	if err != nil || len(got) != 2 || got[0].Path != "/dev/nvme0n1" || got[1].Path != "/dev/sdb" {
		t.Fatalf("got %+v %v", got, err)
	}
	if s := got[0].String(); s != "/dev/nvme0n1 (512 GB)" {
		t.Fatalf("String() = %q", s)
	}
}
