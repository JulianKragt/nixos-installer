package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingReturnsFreshState(t *testing.T) {
	st, err := Load(t.TempDir(), "atlas")
	if err != nil {
		t.Fatal(err)
	}
	if st.HostName != "atlas" || len(st.CompletedStages) != 0 {
		t.Errorf("state: %+v", st)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	want := &State{HostName: "atlas", Target: "10.0.0.2", Disk: "/dev/vda", CompletedStages: []string{"1.0"}}
	if err := want.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, "atlas")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	info, err := os.Stat(filepath.Join(dir, "atlas.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("state file: %v, %v", info, err)
	}
}

func TestLoadCorruptFileFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "atlas.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, "atlas"); err == nil {
		t.Error("want error for corrupt state")
	}
}
