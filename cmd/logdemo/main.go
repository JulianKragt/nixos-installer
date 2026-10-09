// Command logdemo shows the look of internal/runner. Dev-only.
//
//	go run ./cmd/logdemo [-verbose]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"installer/internal/runner"
	"os"
	"time"
)

func main() {
	verbose := flag.Bool("verbose", false, "show debug tasks and messages")
	flag.Parse()

	logger := runner.New(runner.Options{Out: os.Stderr, Verbose: *verbose})
	ctx := runner.NewContext(context.Background(), logger)

	failed := demo(ctx)
	logger.Close()
	if failed {
		os.Exit(1)
	}
}

func sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func demo(ctx context.Context) bool {
	runner.Info(ctx, "Starting demo")

	_ = runner.Run(ctx, "Provider preparation", func(ctx context.Context) error {
		sleep(600)
		runner.Info(ctx, "target reachable", "addr", "192.168.1.100:22")
		runner.Debug(ctx, "debug detail", "ssh", "ok")
		_ = runner.Run(ctx, "Evaluating install spec", func(ctx context.Context) error {
			sleep(800)
			return nil
		})
		return nil
	})

	_ = runner.Run(ctx, "Base install", func(ctx context.Context) error {
		_ = runner.Run(ctx, "Copying closure", func(ctx context.Context) error {
			sleep(900)
			return nil
		})
		return runner.Run(ctx, "Partitioning disk", func(ctx context.Context) error {
			out := runner.Output(ctx)
			for _, l := range []string{
				"Creating new GPT entries",
				"Formatting /dev/nvme0n1p2",
				"\033[32mmkfs.ext4\033[0m: writing inode tables",
				"progress 10%\rprogress 50%\rprogress 100%",
				"Syncing",
				"Done",
			} {
				fmt.Fprintln(out, l)
				sleep(500)
			}
			return nil
		})
	})

	_ = runner.Run(ctx, "Parallel checks", func(ctx context.Context) error {
		return runner.Parallel(ctx,
			runner.Step{Title: "Checking GitHub reachability", Fn: func(ctx context.Context) error { sleep(1800); return nil }},
			runner.Step{Title: "Checking cache reachability", Fn: func(ctx context.Context) error { sleep(1100); return nil }},
			runner.Step{Title: "Checking DNS", Fn: func(ctx context.Context) error {
				_, t := runner.StartDebug(ctx, "resolver probe")
				defer t.Done()
				sleep(700)
				return nil
			}},
		)
	})

	err := runner.Run(ctx, "Failing stage", func(ctx context.Context) error {
		out := runner.Output(ctx)
		for i := 1; i <= 8; i++ {
			fmt.Fprintf(out, "step %d of 8\n", i)
			sleep(200)
		}
		runner.Warn(ctx, "about to fail")
		return runner.Parallel(ctx,
			runner.Step{Title: "Slow sibling", Fn: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(5 * time.Second):
					return nil
				}
			}},
			runner.Step{Title: "Failing sibling", Fn: func(ctx context.Context) error {
				sleep(500)
				return errors.New("exit status 2")
			}},
		)
	})
	return err != nil
}
