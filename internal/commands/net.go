package commands

import (
	"context"
	"fmt"
	"installer/internal/command"
	"installer/internal/command/executor"
	"net"
	"strings"
)

// DialTCP checks TCP connectivity to addr ("host:port") by running nc -z.
// Works with any executor — pass env.Local to check from the provider machine,
// or env.Remote to check from the receiver machine.
func DialTCP(ctx context.Context, exec executor.Executor, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid addr %s: %w", addr, err)
	}
	result, err := exec.Run(ctx, command.New("nc", "-z", "-w", "5", host, port), executor.ExecOptions{})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("unreachable %s: %s", addr, strings.TrimSpace(result.Output))
	}
	return nil
}

// CheckSSH verifies that a command can be run on the machine behind exec,
// i.e. that the connection and authentication work.
func CheckSSH(ctx context.Context, exec executor.Executor) error {
	result, err := exec.Run(ctx, command.New("true"), executor.ExecOptions{})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("remote check failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Output))
	}
	return nil
}
