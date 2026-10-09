package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"installer/internal/cli"
	"installer/internal/command/executor"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"installer/internal/stage"
	"installer/internal/stages"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	go func() { // restore default handling so a second Ctrl+C force-quits
		<-ctx.Done()
		stop()
	}()

	opts, err := cli.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	stateDir := appStateDir()
	logOpts := runner.Options{Out: os.Stderr, Verbose: opts.Verbose}
	logPath, logFile, err := openLogFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: no log file: %v\n", err)
	} else {
		defer logFile.Close()
		logOpts.File = logFile
	}

	logger := runner.New(logOpts)
	ctx = runner.NewContext(ctx, logger)

	in := runner.NewInput(os.Stdin)
	st, err := resolveInputs(ctx, &opts, in, stateDir)
	if err != nil {
		logger.Close()
		if logFile != nil {
			fmt.Fprintf(os.Stderr, "Full log: %s\n", logPath)
		}
		return 1
	}

	home, _ := os.UserHomeDir()
	sshKey := filepath.Join(home, ".ssh", "id_ed25519")
	remote := &stage.TargetSSH{SSH: executor.SSH{User: "root", KeyPath: sshKey}, State: st}
	env := &stage.Env{State: st, Local: streaming{executor.NewLocal()}, Remote: streaming{remote}}
	p := pipeline.New([]stage.Stage{
		&stages.EnvironmentPreparationStage{
			Opts:     opts,
			SSHKey:   sshKey,
			StateDir: stateDir,
			In:       in,
		},
	}, env, stateDir)
	runErr := p.Run(ctx)

	logger.Close()
	if runErr != nil {
		if logFile != nil {
			fmt.Fprintf(os.Stderr, "Full log: %s\n", logPath)
		}
		return 1
	}
	return 0
}

// appStateDir is the installer's state directory.
func appStateDir() string { return xdgStateDir("nixos-installer") }

// xdgStateDir returns ${XDG_STATE_HOME:-~/.local/state}/<app>.
func xdgStateDir(app string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", ".state", app)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, app)
}

// openLogFile creates ${XDG_STATE_HOME:-~/.local/state}/nixos-installer/logs/install-<time>.log.
func openLogFile() (string, *os.File, error) {
	dir := filepath.Join(appStateDir(), "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("install-%s.log", time.Now().Format("20060102-150405")))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return "", nil, err
	}
	return path, f, nil
}
