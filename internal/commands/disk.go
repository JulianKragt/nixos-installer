package commands

import (
	"context"
	"fmt"
	"installer/internal/command"
	"installer/internal/command/executor"
	"strconv"
	"strings"
)

// Disk is a whole block device that can be installed to.
type Disk struct {
	Path string
	Size int64 // bytes
}

func (d Disk) String() string { return fmt.Sprintf("%s (%d GB)", d.Path, d.Size/1e9) }

// ListDisks returns the non-removable whole disks of the machine behind exec.
func ListDisks(ctx context.Context, exec executor.Executor) ([]Disk, error) {
	out, err := runOK(ctx, exec, command.New("lsblk", "-dnb", "-o", "NAME,SIZE,TYPE,RM"), "lsblk failed")
	if err != nil {
		return nil, err
	}
	return parseLsblk(out), nil
}

// parseLsblk keeps non-removable devices of type "disk" from
// `lsblk -dnb -o NAME,SIZE,TYPE,RM` output.
func parseLsblk(out string) []Disk {
	var disks []Disk
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 4 || f[2] != "disk" || f[3] != "0" {
			continue
		}
		size, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || size == 0 {
			continue
		}
		disks = append(disks, Disk{Path: "/dev/" + f[0], Size: size})
	}
	return disks
}
