package stage

import (
	"context"
	"errors"
	"installer/internal/command"
	"installer/internal/command/executor"
	"installer/internal/state"
	"net"
)

// TargetSSH is an executor for the machine at State.Target. The address is
// read on every call, so it can be constructed before the target is known.
type TargetSSH struct {
	SSH   executor.SSH
	State *state.State
}

func (t *TargetSSH) addr() (string, error) {
	if t.State.Target == "" {
		return "", errors.New("no target address resolved yet")
	}
	return net.JoinHostPort(t.State.Target, executor.DefaultSSHPort), nil
}

func (t *TargetSSH) Run(ctx context.Context, cmd command.Command, opts executor.ExecOptions) (command.Result, error) {
	addr, err := t.addr()
	if err != nil {
		return command.Result{}, err
	}
	ssh := t.SSH // per-call copy: calls may run concurrently
	ssh.Host = addr
	return ssh.Run(ctx, cmd, opts)
}
