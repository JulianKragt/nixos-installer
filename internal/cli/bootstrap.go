// Package cli implements the top-level subcommands: install and secrets.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Options are the resolved command-line inputs.
type Options struct {
	Verbose   bool
	ConfigDir string // path to the nixos-config flake
	Host      string // empty = pick from ListHosts
	TargetIP  string
	Disk      string // empty = detect on the target
}

// Parse parses args (without the program name). Usage and flag errors are
// written to stderr; the returned error is already user-presentable.
func Parse(args []string, stderr io.Writer) (Options, error) {
	var o Options
	fs := flag.NewFlagSet("nixos-installer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&o.Verbose, "verbose", false, "Enable verbose logging")
	fs.StringVar(&o.ConfigDir, "config", "./", "Path to the nixos-config flake")
	fs.StringVar(&o.Host, "host", "", "Host to install (prompts when empty)")
	fs.StringVar(&o.TargetIP, "target", "", "IP address of the target device (required)")
	fs.StringVar(&o.Disk, "disk", "", "Disk to install to, e.g. /dev/nvme0n1 (detected on the target when empty)")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if o.TargetIP != "" {
		if err := validateTarget(o.TargetIP); err != nil {
			return Options{}, err
		}
	}
	return o, nil
}

func validateTarget(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("--target: %q is not a valid IP address", ip)
	}
	return nil
}

// Asker shows prompt and returns the operator's answer.
type Asker func(prompt string) (string, error)

// CompleteTarget asks for the target IP when it is still empty, until it is valid.
func (o *Options) CompleteTarget(ask Asker) error {
	prompt := "Target IP address: "
	for o.TargetIP == "" {
		line, err := ask(prompt)
		if line == "" && err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("--target is required")
			}
			return err
		}
		if verr := validateTarget(line); verr != nil {
			prompt = fmt.Sprintf("Target IP address (%q is not a valid IP): ", line)
			continue
		}
		o.TargetIP = line
	}
	return nil
}

// ListHosts returns the hosts that are valid install targets: directories
// under <configDir>/hosts/nixos that contain bootstrap.nix.
func ListHosts(configDir string) ([]string, error) {
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
	sort.Strings(hosts)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no installable hosts found in %s", root)
	}
	return hosts, nil
}

// PickHost resolves the host to install. A non-empty want must be in hosts;
// otherwise the operator chooses from a numbered list.
func PickHost(ask Asker, hosts []string, want string) (string, error) {
	if want != "" {
		for _, h := range hosts {
			if h == want {
				return h, nil
			}
		}
		return "", fmt.Errorf("unknown host %q (available: %s)", want, strings.Join(hosts, ", "))
	}
	if len(hosts) == 1 {
		return hosts[0], nil
	}
	var b strings.Builder
	b.WriteString("Select a host to install:\n")
	for i, h := range hosts {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, h)
	}
	b.WriteString("Host number: ")
	line, err := ask(b.String())
	if line == "" && err != nil {
		return "", fmt.Errorf("read selection: %w", err)
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(hosts) {
		return "", fmt.Errorf("invalid selection %q", line)
	}
	return hosts[n-1], nil
}
