package commands

import (
	"context"
	"fmt"
	"installer/internal/command"
	"installer/internal/command/executor"
	"net"
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
		return fmt.Errorf("unreachable %s", addr)
	}
	return nil
}
