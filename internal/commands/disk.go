package commands

import (
	"context"
	"errors"
	"fmt"
	"installer/internal/command"
	"installer/internal/command/executor"
	"installer/internal/runner"
	"strconv"
	"strings"
)

type blockDevice struct {
	path string
	size int64
}

// DetectDisk finds the install disk on the machine behind exec: the largest
// non-removable whole disk. It logs all candidates so the operator can verify.
func DetectDisk(ctx context.Context, exec executor.Executor) (string, error) {
	result, err := exec.Run(ctx, command.New("lsblk", "-dnb", "-o", "NAME,SIZE,TYPE,RM"), executor.ExecOptions{})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("lsblk failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Output))
	}
	disks := parseLsblk(result.Output)
	if len(disks) == 0 {
		return "", errors.New("no installable disk found on target")
	}
	best := disks[0]
	for _, d := range disks[1:] {
		if d.size > best.size {
			best = d
		}
	}
	if len(disks) > 1 {
		names := make([]string, len(disks))
		for i, d := range disks {
			names[i] = d.path
		}
		runner.Warn(ctx, fmt.Sprintf("Multiple disks found (%s); using largest: %s", strings.Join(names, ", "), best.path))
	}
	return best.path, nil
}

// parseLsblk keeps non-removable devices of type "disk" from
// `lsblk -dnb -o NAME,SIZE,TYPE,RM` output.
func parseLsblk(out string) []blockDevice {
	var disks []blockDevice
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 4 || f[2] != "disk" || f[3] != "0" {
			continue
		}
		size, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || size == 0 {
			continue
		}
		disks = append(disks, blockDevice{path: "/dev/" + f[0], size: size})
	}
	return disks
}
