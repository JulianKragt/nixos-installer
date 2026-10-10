package main

import (
	"context"
	"errors"
	"fmt"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// resolveInputs asks for whatever the flags left empty (target, host) and
// loads the persisted state of the chosen host, since state is keyed by host
// name. in is completed in place.
func resolveInputs(ctx context.Context, in *pipeline.Inputs, stateDir string) (*pipeline.State, error) {
	var st *pipeline.State
	err := runner.Run(ctx, "Checking inputs", func(ctx context.Context) (err error) {
		if in.Target, err = askTarget(ctx, in.Target); err != nil {
			return err
		}
		hosts, err := listHosts(in.ConfigDir)
		if err != nil {
			return err
		}
		if in.Host, err = pickHost(ctx, hosts, in.Host); err != nil {
			return err
		}
		if st, err = pipeline.LoadState(stateDir, in.Host); err != nil {
			return err
		}
		runner.Info(ctx, "Target: "+in.Target)
		runner.Info(ctx, "Host: "+in.Host)
		return nil
	})
	return st, err
}

// askTarget returns target, or asks for the target IP until the answer is
// valid when target is empty.
func askTarget(ctx context.Context, target string) (string, error) {
	prompt := "Target IP address: "
	for target == "" {
		line, err := runner.Ask(ctx, prompt)
		if errors.Is(err, io.EOF) {
			return "", errors.New("--target is required")
		}
		if err != nil {
			return "", err
		}
		if validateTarget(line) != nil {
			prompt = fmt.Sprintf("Target IP address (%q is not a valid IP): ", line)
			continue
		}
		target = line
	}
	return target, nil
}

// listHosts returns the hosts that are valid install targets: directories
// under <configDir>/hosts/nixos that contain bootstrap.nix.
func listHosts(configDir string) ([]string, error) {
	root := filepath.Join(configDir, "hosts", "nixos")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	var hosts []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "bootstrap.nix")); err == nil {
			hosts = append(hosts, e.Name())
		}
	}
	slices.Sort(hosts)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no installable hosts found in %s", root)
	}
	return hosts, nil
}

// pickHost resolves the host to install. A non-empty want must be in hosts;
// otherwise the operator chooses from a numbered list.
func pickHost(ctx context.Context, hosts []string, want string) (string, error) {
	if want != "" {
		if slices.Contains(hosts, want) {
			return want, nil
		}
		return "", fmt.Errorf("unknown host %q (available: %s)", want, strings.Join(hosts, ", "))
	}
	if len(hosts) == 1 {
		return hosts[0], nil
	}
	i, err := runner.Choose(ctx, "Select a host to install:", hosts)
	if err != nil {
		return "", err
	}
	return hosts[i], nil
}
