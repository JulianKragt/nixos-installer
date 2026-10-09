package executor

import (
	"context"
	"installer/internal/command"
	"testing"
)

func TestSSHRejectsBadSetup(t *testing.T) {
	ctx := context.Background()
	if _, err := (&SSH{}).Run(ctx, command.New("true"), ExecOptions{}); err == nil {
		t.Error("empty host should fail instead of dialing localhost")
	}
}
