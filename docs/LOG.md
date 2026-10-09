# `internal/log` — how to log in this repo

Read this before writing or changing any code that prints progress. Do not use `fmt.Print*` or `os.Stderr` for progress; do not add a second logger.

## Idea

The logger keeps a **tree of tasks**. The current task travels in the `context.Context`, so any code that receives `ctx` logs under the correct parent. This is the OpenTelemetry span pattern. If `ctx` has no logger, every call is a no-op (safe in tests).

On a TTY, finished top-level (root) tasks are printed permanently; running ones stay in a live area that is redrawn every 80 ms. Headers stay on top, their spinner turns into `✓`/`✗` in place, several tasks can spin at once. A non-TTY `Out` only gets finished blocks, no colors, no escape codes.

## API

```go
import "installer/internal/log"

// main only
l := log.New(log.Options{Out: os.Stderr, File: f, Verbose: v})
ctx = log.NewContext(ctx, l)
defer/before os.Exit: l.Close()        // must run before os.Exit, restores the cursor

// Tasks
log.Run(ctx, "Title", func(ctx context.Context) error {...}) error   // preferred
ctx, t := log.Start(ctx, "Title"); defer t.End(&err)                  // manual; also t.Done(), t.Fail(err)
ctx, t := log.StartDebug(ctx, "Title")      // hidden (with children) unless Verbose

// Parallel: each step is its own child task, first error cancels siblings,
// errors are joined (sibling-cancel errors are dropped).
log.Parallel(ctx, log.Step{Title: "A", Fn: func(ctx context.Context) error {...}}, ...)

// Messages go into ctx's task
log.Debug/Info/Warn/Error(ctx, "msg", "key", val)   // key/value pairs, or Sprintf if msg contains %

// Command output (tail under the task, full text in the log file)
io.MultiWriter(&buf, log.Output(ctx))
```

## Rules

- **Always pass the `ctx` returned by `Start`/`Run`/`Parallel`** into the work and into goroutines. Logging with the parent's ctx puts lines under the wrong task.
- Logging goes through `ctx` only. `Task` handles lifecycle only.
- Pipeline already wraps each stage in `log.Run(ctx, stage.Name(), ...)`. A stage must **not** create its own header; use `log.Run` for sub-steps.
- `go test -race` is required. Parallel steps must not write `env.State`: return results, assign after the join.
- Commands that handle secrets must not stream output via `log.Output` (no opt-out exists yet; add one with the first such command).
- Debug level: `Debug` lines and `StartDebug` tasks show only with `--verbose`, but always go to the log file.

## Behaviour worth knowing

- Roots are committed in start order. A root that finishes early stays live until all earlier roots are committed.
- Ending a task closes still-running children as `✗ … (parent ended)`.
- Output tail: 5 lines shown under a running task, dropped on success; on failure the last 20 lines are kept under the `✗`.
- Output writer: buffers partial lines, `\r` resets the line, strips ANSI/control chars.
- Live area taller than the terminal: header, `… N lines`, newest lines; the commit prints everything.
- Log file: `${XDG_STATE_HOME:-~/.local/state}/nixos-installer/logs/<host>-<time>.log`, debug level, no ANSI, format `15:12:03.501 │ Base install › Partitioning disk | message`. `main` prints `Full log: <path>` on failure.
- GoLand's console is not a TTY and paints stderr red; enable "Emulate terminal in output console" to see the live view.

## Files

`log.go` Logger/Options/ctx/level funcs · `task.go` nodes, Start/Run/End/Parallel · `tty.go` ticker, commit, layout · `output.go` command-output writer · `file.go` file sink · `format.go` formatting helpers, colors, spinner frames.

Tests build a Logger with `newLogger(opts, tty, size)`, a fake clock (`l.now`) and call `l.draw()` directly (see `log_test.go` for the fake terminal). `go run ./cmd/logdemo [-verbose]` shows the look.

Not built yet (design leaves room): full plain/CI mode, `Pause()` for interactive prompts, SSH executor.
