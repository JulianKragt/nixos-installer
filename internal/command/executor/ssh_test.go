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
	if _, err := (&SSH{Host: "192.0.2.1"}).Run(ctx, command.New("true"), ExecOptions{Dir: "/tmp"}); err == nil {
		t.Error("Dir is not supported and should fail loudly")
	}
}
