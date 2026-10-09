// Command logdemo shows the look of internal/log. Dev-only.
//
//	go run ./cmd/logdemo [-verbose]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"installer/internal/log"
	"os"
	"time"
)

func main() {
	verbose := flag.Bool("verbose", false, "show debug tasks and messages")
	flag.Parse()

	logger := log.New(log.Options{Out: os.Stderr, Verbose: *verbose})
	ctx := log.NewContext(context.Background(), logger)

	failed := demo(ctx)
	logger.Close()
	if failed {
		os.Exit(1)
	}
}

func sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func demo(ctx context.Context) bool {
	log.Info(ctx, "Starting demo")

	_ = log.Run(ctx, "Provider preparation", func(ctx context.Context) error {
		sleep(600)
		log.Info(ctx, "target reachable", "addr", "192.168.1.100:22")
		log.Debug(ctx, "debug detail", "ssh", "ok")
		_ = log.Run(ctx, "Evaluating install spec", func(ctx context.Context) error {
			sleep(800)
			return nil
		})
		return nil
	})

	_ = log.Run(ctx, "Base install", func(ctx context.Context) error {
		_ = log.Run(ctx, "Copying closure", func(ctx context.Context) error {
			sleep(900)
			return nil
		})
		return log.Run(ctx, "Partitioning disk", func(ctx context.Context) error {
			out := log.Output(ctx)
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

	_ = log.Run(ctx, "Parallel checks", func(ctx context.Context) error {
		return log.Parallel(ctx,
			log.Step{Title: "Checking GitHub reachability", Fn: func(ctx context.Context) error { sleep(1800); return nil }},
			log.Step{Title: "Checking cache reachability", Fn: func(ctx context.Context) error { sleep(1100); return nil }},
			log.Step{Title: "Checking DNS", Fn: func(ctx context.Context) error {
				_, t := log.StartDebug(ctx, "resolver probe")
				defer t.Done()
				sleep(700)
				return nil
			}},
		)
	})

	err := log.Run(ctx, "Failing stage", func(ctx context.Context) error {
		out := log.Output(ctx)
		for i := 1; i <= 8; i++ {
			fmt.Fprintf(out, "step %d of 8\n", i)
			sleep(200)
		}
		log.Warn(ctx, "about to fail")
		return log.Parallel(ctx,
			log.Step{Title: "Slow sibling", Fn: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(5 * time.Second):
					return nil
				}
			}},
			log.Step{Title: "Failing sibling", Fn: func(ctx context.Context) error {
				sleep(500)
				return errors.New("exit status 2")
			}},
		)
	})
	return err != nil
}
