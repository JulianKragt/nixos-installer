package pipeline

import (
	"bytes"
	"context"
	"installer/internal/log"
	"installer/internal/stage"
	"installer/internal/state"
	"strings"
	"testing"
)

type fakeStage struct {
	idx  int
	name string
	ran  *[]string
}

func (f fakeStage) Index() int   { return f.idx }
func (f fakeStage) Name() string { return f.name }
func (f fakeStage) Run(ctx context.Context, env *stage.Env) error {
	*f.ran = append(*f.ran, f.name)
	return nil
}
func (f fakeStage) Rollback(ctx context.Context, env *stage.Env) error { return nil }

func TestRunsOnlyPendingStagesInOrder(t *testing.T) {
	var ran []string
	stages := []stage.Stage{
		fakeStage{3, "c", &ran},
		fakeStage{1, "a", &ran},
		fakeStage{2, "b", &ran},
	}
	var out bytes.Buffer
	ctx := log.NewContext(context.Background(), log.New(log.Options{Out: &out}))
	p := New(stages, &stage.Env{State: &state.State{StageIndex: 1}})
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ran, ","); got != "b,c" {
		t.Fatalf("ran %s, want b,c", got)
	}
}
