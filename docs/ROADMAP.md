# Roadmap: the Go installer replaces `install-host.sh`

Companion to [CONTEXT.md](CONTEXT.md). Each row in "Order of work" has a prompt in [prompts/](prompts/) that you can give to a fresh chat to get a specific plan for that stage.

## Goal

Replace `config/scripts/install-host.sh` and `just install | enroll-sops | deploy-remote` with the Go installer. Spec for "what": the stage table in `docs/CONTEXT.md` §5 (the Pipeline doc itself is not in any repo). Reference for "how": the bash script.

## State of things (2026-10-10)

`docs/CONTEXT.md` §1, §4 and §7 describe the installer as it is now. §3 (nix-secrets) and §6 (open decisions) are stale; the points below and "Decisions" replace them.

- **installer**: stages are plain functions listed in `stages.All`; `pipeline.Machine` runs a command on the provider or, through the system `ssh`, on the target; `pipeline.State` (`Host`, `Disk`, `Completed`) is one JSON file per host. Only stage 0.0 exists. `pipeline.SSH` still disables host key checking (TODO in `internal/pipeline/machine.go`). Build and tests are green.
- **nix-secrets**: already on `hosts/<host>.yaml` + `host-users/<host>-<user>.yaml`; `.sops.yaml` deduped with anchored regexes; `shared.yaml` and `users/` are gone. No entries for `broadway` or `media`. `hosts/workhorse.yaml` is 0 bytes. `templates/user.yaml` = `hashedPassword` only.
- **config**: reads `host-users/…`. User SSH keys no longer come from secrets, so the only per-user secret is `hashedPassword`, decrypted by system sops-nix with the host key. 19 files are uncommitted (provisionSpec → `installHosts`); `nix eval --json .#installHosts` works; `install-host.sh` still reads the removed `provisionSpec`, so **the bash path no longer runs**.
- **provider (workhorse)**: has `go` (`~/sdk/go1.27.1/bin/go`), `nix`, `git`, `ssh`; no `sops`, `age`, `ssh-to-age` in PATH.

## Decisions (settled)

| Topic | Decision |
|---|---|
| §6.2 who decrypts | Host key decrypts `host-users/*`; no operator key on the target. Legacy Phase 7.5 is not ported. |
| §6.3 target git auth | SSH agent forwarding (`ssh -A`), opt-in per command on `pipeline.Machine`. Nothing is written to the target; the `git+ssh://` flake URL stays. |
| §6.4 lock + rebuild | Provider updates, commits and pushes `flake.lock`; target runs `nixos-rebuild switch --flake git+ssh://…/nixos-config?rev=<ConfigRev>#<host>`. Only committed + pushed config is installed. |
| §6.5 user recipients | `.sops.yaml` anchors are the source of truth. `host-users/<h>-<u>.yaml` = `*host_<h>` + `*user_<u>` if that anchor exists, else `*user_<primaryUser>`. Missing `&user_<primaryUser>` → fail closed. `hosts/<h>.yaml` stays host-only. |
| §6.6 reinstall | On `&host_<h>` mismatch: show old/new, typed confirmation, replace the anchor, re-author files the new host key cannot decrypt from templates. |
| §6.7 shared file | Moot: `shared.yaml` no longer exists. |
| Disk | Installer writes `hosts/nixos/<host>/install.json` (`{"disk": …}`); `disko.nix` reads it; committed with `hardware-configuration.nix`. |

## Order of work

There is no separate plumbing milestone: shared plumbing is built by the first stage that needs it.

| # | Stage | Runs on | Needs | Introduces shared plumbing | Writes State | Prompt |
|---|---|---|---|---|---|---|
| – | Prep | provider | – | – | – | [00-prep.md](prompts/00-prep.md) |
| – | Config prerequisites | `~/src/config` | – | – | – | [config-prerequisites.md](prompts/config-prerequisites.md) |
| 0.1 | Provider preparation | provider (+target checks) | 0.0, B1 | `Machine` run without output streaming, `runner.AskSecret`, `nix` and `git` helpers | `PrimaryUser`, `Users` | [0.1-provider-preparation.md](prompts/0.1-provider-preparation.md) |
| 0.2 | Base install | provider → target | 0.1 | host key pinning in `pipeline.SSH` | `HostKeyFingerprint` | [0.2-base-install.md](prompts/0.2-base-install.md) |
| 1.1 | Host identity | target | 0.2 | ssh→age in-process | `HostRecipient` | [1.1-host-identity.md](prompts/1.1-host-identity.md) |
| 1.2 | Readiness | target | 1.1, B2 | – | `Marker` | [1.2-readiness.md](prompts/1.2-readiness.md) |
| 2.0 | Auth bootstrap | target | 1.2 | agent forwarding on `Machine` | `SecretsURL` | [2.0-auth-bootstrap.md](prompts/2.0-auth-bootstrap.md) |
| 2.1 | Sync repo | target | 2.0 | – | – | [2.1-sync-repo.md](prompts/2.1-sync-repo.md) |
| 2.2 | Recipients | target | 2.1 | structural `.sops.yaml` editor | – | [2.2-recipients.md](prompts/2.2-recipients.md) |
| 2.3 | Template authoring | target | 2.2 | interactive `ssh -t` + runner hand-over, secret-file checks | – | [2.3-template-authoring.md](prompts/2.3-template-authoring.md) |
| 2.4 | Encrypt/commit/push | target | 2.3 | – | `SecretsCommit` | [2.4-commit-push.md](prompts/2.4-commit-push.md) |
| 3.1 | Flake input | provider | 2.4 | – | `ConfigRev` | [3.1-flake-input.md](prompts/3.1-flake-input.md) |
| 3.2 | Rebuild | target | 3.1, B3 | – | – | [3.2-rebuild.md](prompts/3.2-rebuild.md) |
| 3.3 | Cleanup | target | 3.2 | – | `Marker` | [3.3-cleanup.md](prompts/3.3-cleanup.md) |
| 4.1 | Policy | provider | 2.4 | – | – | [4.1-policy.md](prompts/4.1-policy.md) |
| 4.2 | Recovery (CLI) | provider | all | subcommands, re-run of done stages | – | [4.2-recovery-cli.md](prompts/4.2-recovery-cli.md) |
| – | Retire bash, package | both repos | all | – | – | [retire-bash.md](prompts/retire-bash.md) |

## Prerequisites outside this repo

- **B1 (before 0.1)** config: commit the pending `installHosts` work; each `disko.nix` reads `./install.json`; add `install.json` for atlas.
- **B2 (before 1.2)** config `modules/bootstrap-base.nix`: packages git, sops, age, ssh-to-age, vim; `nix.settings.experimental-features` = nix-command, flakes, pipe-operators; `programs.ssh.knownHosts` for github.com.
- **B3 (before 3.2)** config: remove `users.users.root.initialPassword = "test"`; remove the `sopsUserAgeKeys` pending-key script; github.com known host in the full config.
- **nix-secrets**: fix or delete the empty `hosts/workhorse.yaml`; `&user_media` or rely on the primary-user fallback before installing broadway.

## Rules for every stage

- CONTEXT §4 "Adding a stage" and "Conventions" apply: a stage is a plain function in `internal/stages/NN_name.go` plus one entry in `stages.All`; no interface without two implementations; no new package unless a second stage needs the code; `internal/stages` holds stage files only.
- Secrets go through `RunStdin`, never into args. A command that can print a secret runs without output streaming, so nothing reaches the runner or the log file. Remote scripts run with `umask 077`.
- Idempotent and safe to re-run. Git commits only for listed paths and only when something changed.
- A stage has no own header (`pipeline.Run` wraps it); sub-steps use `runner.Run` / `runner.Parallel`; parallel steps return values and never write `env.State`.
- A fact a later run needs is a field on `pipeline.State`, added by the stage that first writes it. A new flag is a field on `pipeline.Inputs` and a line in `cmd/nixos-installer/flags.go`.
- `go build ./... && go vet ./... && go test -race ./...` green after every stage.

## End-to-end check

`go run ./cmd/nixos-installer --config ../config --host <h> --target <ip>` against a target booted from the NixOS ISO; run it a second time and confirm nothing is redone and no new commits appear in nix-secrets or config. A throwaway VM host in config (same CPU architecture as the Mac) avoids wiping atlas. Final acceptance: fresh install, `secrets` run after adding and after removing a `home/<u>/<host>.nix`, and a reinstall of the same host.
