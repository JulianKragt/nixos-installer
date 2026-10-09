package stage

import (
	"installer/internal/state"
	"testing"
)

func TestTargetSSHAddr(t *testing.T) {
	st := &state.State{}
	ts := &TargetSSH{State: st}
	if _, err := ts.addr(); err == nil {
		t.Error("want error before a target is set")
	}
	for target, want := range map[string]string{
		"192.168.1.100": "192.168.1.100:22",
		"2001:db8::5":   "[2001:db8::5]:22",
	} {
		st.Target = target
		if got, err := ts.addr(); err != nil || got != want {
			t.Errorf("target %s: addr = %q, %v; want %q", target, got, err, want)
		}
	}
}
