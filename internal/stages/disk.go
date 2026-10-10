package stages

import (
	"context"
	"errors"
	"fmt"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"strconv"
	"strings"
)

// Disk is a whole block device that can be installed to.
type Disk struct {
	Path string
	Size int64 // bytes
}

func (d Disk) String() string { return fmt.Sprintf("%s (%d GB)", d.Path, d.Size/1e9) }

// listDisks returns the non-removable whole disks of m.
func listDisks(ctx context.Context, m pipeline.Machine) ([]Disk, error) {
	out, err := m.Run(ctx, "lsblk", "-dnb", "-o", "NAME,SIZE,TYPE,RM")
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

// chooseDisk picks the install disk among the candidates found on the target.
// The disk recorded by an earlier run wins, a lone candidate is taken as is and
// with several the operator chooses. Without candidates the recorded disk stays.
func chooseDisk(ctx context.Context, candidates []Disk, recorded string) (string, error) {
	for _, d := range candidates {
		if d.Path == recorded {
			return recorded, nil
		}
	}
	switch len(candidates) {
	case 0:
		if recorded == "" {
			return "", errors.New("no installable disk found on target; use --disk")
		}
		return recorded, nil
	case 1:
		return candidates[0].Path, nil
	}
	items := make([]string, len(candidates))
	for i, d := range candidates {
		items[i] = d.String()
	}
	i, err := runner.Choose(ctx, "Select the install disk:", items)
	if err != nil {
		return "", fmt.Errorf("%w (or pass --disk)", err)
	}
	return candidates[i].Path, nil
}
