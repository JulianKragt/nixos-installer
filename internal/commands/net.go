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
	_, err = runOK(ctx, exec, command.New("nc", "-z", "-w", "5", host, port), "unreachable "+addr)
	return err
}

// CheckSSH verifies that a command can be run on the machine behind exec,
// i.e. that the connection and authentication work.
func CheckSSH(ctx context.Context, exec executor.Executor) error {
	_, err := runOK(ctx, exec, command.New("true"), "remote check failed")
	return err
}
