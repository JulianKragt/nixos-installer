package main

import (
	"context"
	"flag"
	"fmt"
	"installer/internal/command/executor"
	"installer/internal/runner"
	"installer/internal/pipeline"
	"installer/internal/stage"
	"installer/internal/stages"
	"installer/internal/state"
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

	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	st := &state.State{
		HostName: "atlas",
		Target:   "192.168.1.100",
	}

	opts := runner.Options{Out: os.Stderr, Verbose: *verbose}
	logPath, logFile, err := openLogFile(st.HostName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: no log file: %v\n", err)
	} else {
		defer logFile.Close()
		opts.File = logFile
	}

	logger := runner.New(opts)
	ctx = runner.NewContext(ctx, logger)

	env := &stage.Env{State: st, Local: executor.NewLocal()}
	p := pipeline.New([]stage.Stage{stages.ProviderPreparationStage{}}, env)
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

// openLogFile creates ${XDG_STATE_HOME:-~/.local/state}/nixos-installer/logs/<host>-<time>.log.
func openLogFile(host string) (string, *os.File, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil, err
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "nixos-installer", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.log", host, time.Now().Format("20060102-150405")))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return "", nil, err
	}
	return path, f, nil
}
