package commands

import (
	"context"
	"fmt"
	"installer/internal/command"
	"installer/internal/command/executor"
	"strings"
)

// runOK runs cmd on exec and returns its output. A non-zero exit code becomes
// an error of the form "<what> (exit N): <output>".
func runOK(ctx context.Context, exec executor.Executor, cmd command.Command, what string) (string, error) {
	result, err := exec.Run(ctx, cmd, executor.ExecOptions{})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("%s (exit %d): %s", what, result.ExitCode, strings.TrimSpace(result.Output))
	}
	return result.Output, nil
}
