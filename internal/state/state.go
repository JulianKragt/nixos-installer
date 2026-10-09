package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	HostName  string
	Target    string
	ConfigDir string
	Disk      string

	CompletedStages []string
}

// Save writes s as JSON to {dir}/{s.HostName}.json atomically.
func (s *State) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("state: create dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	tmp := filepath.Join(dir, s.HostName+".json.tmp")
	dst := filepath.Join(dir, s.HostName+".json")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("state: write: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("state: rename: %w", err)
	}
	return nil
}

// Load reads {dir}/{hostname}.json into a State. If the file does not exist,
// a fresh State with the given hostname is returned.
func Load(dir, hostname string) (*State, error) {
	path := filepath.Join(dir, hostname+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{HostName: hostname}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("state: unmarshal %s: %w", path, err)
	}
	return &s, nil
}
