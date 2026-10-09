package pipeline

import (
	"bytes"
	"context"
	"installer/internal/runner"
	"installer/internal/stage"
	"installer/internal/state"
	"strings"
	"testing"
)

type fakeStage struct {
	id   string
	name string
	ran  *[]string
}

func (f fakeStage) ID() string   { return f.id }
func (f fakeStage) Name() string { return f.name }
func (f fakeStage) Run(ctx context.Context, env *stage.Env) error {
	*f.ran = append(*f.ran, f.name)
	return nil
}
func (f fakeStage) Rollback(ctx context.Context, env *stage.Env) error { return nil }

func TestRunsOnlyPendingStagesInOrder(t *testing.T) {
	var ran []string
	stages := []stage.Stage{
		fakeStage{"0.3", "c", &ran},
		fakeStage{"0.1", "a", &ran},
		fakeStage{"0.2", "b", &ran},
	}
	var out bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &out}))
	st := &state.State{CompletedStages: []string{"0.1"}}
	p := New(stages, &stage.Env{State: st}, t.TempDir())
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ran, ","); got != "b,c" {
		t.Fatalf("ran %s, want b,c", got)
	}
}
