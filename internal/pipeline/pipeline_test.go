package pipeline

import (
	"bytes"
	"context"
	"installer/internal/runner"
	"installer/internal/stage"
	"installer/internal/state"
	"os"
	"path/filepath"
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

type alwaysStage struct{ fakeStage }

func (alwaysStage) AlwaysRun() bool { return true }

func TestAlwaysRunStageRerunsAndIsNotRecorded(t *testing.T) {
	var ran []string
	stages := []stage.Stage{
		fakeStage{"0.1", "a", &ran},
		alwaysStage{fakeStage{"0.0", "env", &ran}},
	}
	var out bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &out}))
	st := &state.State{CompletedStages: []string{"0.0", "0.1"}}
	p := New(stages, &stage.Env{State: st}, t.TempDir())
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ran, ","); got != "env" {
		t.Fatalf("ran %s, want env", got)
	}
	if got := strings.Join(st.CompletedStages, ","); got != "0.0,0.1" {
		t.Fatalf("completed %s, want unchanged", got)
	}
}

func TestStagesSortNumerically(t *testing.T) {
	var ran []string
	stages := []stage.Stage{
		fakeStage{"10.0", "c", &ran},
		fakeStage{"2.0", "a", &ran},
		fakeStage{"2.10", "b2", &ran},
		fakeStage{"2.9", "b1", &ran},
	}
	var out bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &out}))
	p := New(stages, &stage.Env{State: &state.State{}}, t.TempDir())
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ran, ","); got != "a,b1,b2,c" {
		t.Fatalf("ran %s, want a,b1,b2,c", got)
	}
}

func TestSaveFailureFailsPipeline(t *testing.T) {
	var ran []string
	var out bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &out}))
	// A regular file where the state directory should be makes Save fail.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := New([]stage.Stage{fakeStage{"1.0", "a", &ran}}, &stage.Env{State: &state.State{HostName: "h"}}, filepath.Join(blocker, "state"))
	if err := p.Run(ctx); err == nil {
		t.Fatal("want error when state cannot be saved")
	}
}
