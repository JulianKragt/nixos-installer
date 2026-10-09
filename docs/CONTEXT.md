# Installer Context — what the Go pipeline needs to know

Companion to the **Pipeline** doc. Captures what exists today in the three repos, how it maps onto the pipeline phases, and the gaps/decisions to resolve while building the Go installer stage by stage.

Snapshot taken 2026-10-08 from `~/src/{config,nix-secrets,installer}`.

## Decisions so far

- **The Go installer replaces `config/scripts/install-host.sh` entirely**, including the `just install | enroll-sops | deploy-remote` recipes. The bash script is the reference to port from. When the Go installer can do everything the script does, delete the script and those recipes.
- **nix-secrets will be migrated (by Julian) to the pipeline's canonical paths**: `hosts/<host>.yaml` and `host-users/<host>-<user>.yaml`. The problems listed in §3 get fixed as part of that migration. The installer only targets the new layout; it does not need to support `users/<user>.yaml`.

---

## 1. The three repos at a glance

| Repo | Local path | Remote | Role in pipeline |
|---|---|---|---|
| nixos-config | `~/src/config` | `git@github.com:JulianKragt/nixos-config.git` | Source of host definitions, bootstrap configs, `installSpec`, users. Rebuilt in Phase 3. |
| nix-secrets | `~/src/nix-secrets` | `git@github.com:JulianKragt/nix-secrets.git` (private) | `.sops.yaml` recipients + encrypted YAML. Written in Phase 2. |
| installer | `~/src/installer` | none (not a git repo yet) | Go module `installer`, Go 1.27. Replaces `config/scripts/install-host.sh`. |

`~/src/reference/` holds two third-party configs (EmergentMind, vimjoyer) used as inspiration only.

**Predecessor:** `config/scripts/install-host.sh` (+ `just install|enroll-sops|deploy-remote`) already implements an older version of this flow in bash. It is the best spec for the "how"; the Pipeline doc is the spec for the "what". Note: `config` has *staged, uncommitted* changes to `install-host.sh`, `install-spec.nix`, `secrets.nix`, `README.md`.

---

## 2. nixos-config — what the installer reads

### Flake shape
- flake-parts "dendritic" layout; every fragment is listed explicitly in `imports.nix` (new host files must be added there).
- Each NixOS host exposes **two** outputs:
  - `nixosConfigurations.<host>-bootstrap` → `hosts/nixos/<host>/bootstrap.nix` (minimal: install-spec + openssh + disko + hardware-config)
  - `nixosConfigurations.<host>` → full system (host-spec, core/sops, users, home-manager…)
- Host is a valid install target iff `hosts/nixos/<host>/bootstrap.nix` exists.
- Hosts today: `atlas` (NixOS workstation, `/dev/nvme0n1`, already installed once), `broadway` (NixOS server, disk still `REPLACE_ME`, users jkragt + media), `workhorse` (darwin), `wsl`.

### `installSpec` (modules/install-spec.nix) — install policy per host
Read with `nix eval --json .#nixosConfigurations.<host>-bootstrap.config.installSpec`.

| Option | Default | Use in pipeline |
|---|---|---|
| `hostName`, `primaryUser`, `timeZone`, `stateVersion` | – / – / Europe/Amsterdam / 25.11 | Identity; root authorized_keys come from `accounts/<primaryUser>/keys` + `accounts/super/keys` |
| `generateHardware` | true | `nixos-anywhere --generate-hardware-config nixos-generate-config <path>` → commit in config |
| `hardwareConfigPath` | `hardware-configuration.nix` | relative to `hosts/nixos/<host>/` |
| `enrollSops` | true | Phase 1–2 gate |
| `provisionUserAgeKey` | true | Legacy: streams operator age key to target (see §6.2 — likely obsolete) |
| `pushSecrets` | true | Stage 2.4 gate |
| `deployFullConfig` | true | Stage 3.2 gate |
| `nixSecretsPath` | `../nix-secrets` | Provider-side path (legacy flow edits secrets on provider) |
| `sshWaitTimeout` | 600 (atlas: 500) | Stage 0.2 SSH wait |
| `luksPasswordFile` | `/tmp/disko-password` | Where disko reads the LUKS passphrase on the install ISO |
| `nixosAnywhereExtra` | `[]` | Extra flags for nixos-anywhere |

The bootstrap config also sets systemd-boot and root SSH keys. It does **not** currently include git / sops / age / ssh-to-age / vim (required by Pipeline Stage 0.1 and 1.2).

### Other useful evaluations
- Disk: `nix eval --raw .#nixosConfigurations.<host>-bootstrap.config.disko.devices.disk.main.device` — fail if it contains `REPLACE_ME`.
- Users on a host: a user lives on host iff `home/<user>/<host>.nix` exists. Authoritative: `nix eval --json .#nixosConfigurations.<host>.config.accounts.activeUsers`.
- Primary user: `hostSpec.primaryUser` (full config) / `installSpec.primaryUser` (bootstrap).

### Disk / boot facts that affect the flow
- Disko layout: GPT, 1G ESP, **LUKS** (`cryptroot`) → btrfs subvolumes `@root @home @nix @swap`. Passphrase file `/tmp/disko-password` must exist on the ISO before nixos-anywhere runs.
- No initrd SSH unlock → after reboot (Stage 0.2) someone must type the LUKS passphrase at the console before SSH comes up. The SSH wait timeout covers this.
- Before install the target is a NixOS ISO; legacy flow asks the operator to run `sudo passwd` so root SSH works.

### SSH / host identity
- openssh: ed25519 only, `/etc/ssh/ssh_host_ed25519_key`, `PermitRootLogin = prohibit-password`, no password auth. NixOS generates the host key on first boot → Stage 1.1 mostly *reads* it.
- Legacy flow: TOFU while waiting, then `ssh-keyscan -t ed25519` pins the key into a temp known_hosts with `StrictHostKeyChecking=yes`.

### How the full config consumes secrets (current state)
- Host (NixOS sops-nix, `hosts/common/core/sops.nix`): `defaultSopsFile = ${nix-secrets}/hosts/<host>.yaml`; identity = host SSH key (`age.sshKeyPaths`) + `/var/lib/sops-nix/key.txt`; `validateSopsFiles = false`.
- User passwords (`hosts/common/accounts/secrets.nix`): `sops.secrets."passwords/<u>"` from **`users/<u>.yaml`** key `hashedPassword`, `neededForUsers = true` → `users.users.<u>.hashedPasswordFile`. `users.mutableUsers = false`, so a missing/undecryptable password = no console login.
- Home Manager (`home/common/core/sops.nix`): `defaultSopsFile = users/<u>.yaml`, identity `~/.config/sops/age/keys.txt`, renders `ssh/private_key` → `~/.ssh/id_ed25519`, `ssh/public_key` → `.pub`.
- Activation script `sopsUserAgeKeys` moves `/var/lib/sops-nix/pending-user-age-keys/<u>` into `~/.config/sops/age/keys.txt`.
- Flake input: `nix-secrets = git+ssh://git@github.com/JulianKragt/nix-secrets.git?ref=main&shallow=1`, `flake = false`. Currently locked at rev `5b3f9e7`.

---

## 3. nix-secrets — current layout and state

```
.sops.yaml          keys + creation_rules
shared.yaml         cross-host (only user_jkragt can decrypt)
hosts/<host>.yaml   atlas (encrypted to host_atlas), workhorse (0 bytes!)
users/<user>.yaml   jkragt: ssh.public_key, ssh.private_key, hashedPassword
templates/host.yaml placeholder: change_me
templates/user.yaml hashedPassword, id_ed25519
justfile            edit, edit-user, edit-host, rekey, user-recipient, host-recipient, user-identity, host-identity
flake.nix           devShell: age, sops, ssh-to-age, mkpasswd, just
```

`.sops.yaml` today: anchors `&host_atlas`, `&user_jkragt`, `&host_workhorse`; rules for `shared.yaml`, `hosts/workhorse.yaml`, `users/jkragt.yaml`, and `hosts/atlas\.yaml$` **twice**.

Problems found. Julian is fixing them in the migration to pipeline paths; they are kept here because the installer must not reintroduce them:
- **Duplicate atlas rule**: the bash script checks `grep "hosts/atlas.yaml"` but writes the escaped regex `hosts/atlas\.yaml$`, so the check never matches and every re-run appends. The Go version must edit `.sops.yaml` structurally (e.g. `gopkg.in/yaml.v3` Node API, which preserves anchors, aliases and comments) instead of grepping.
- sops uses the **first matching** creation rule — insert specific host/user rules before any catch-all and dedupe by `path_regex`.
- `hosts/workhorse.yaml` is empty; `hosts/atlas.yaml` has the template comments encrypted (`#ENC[...]`) — strip comments when materializing templates.
- `templates/user.yaml` (`id_ed25519`) does not match what config reads (`ssh/private_key`, `ssh/public_key`, `hashedPassword`). Templates are effectively the schema for Stage 2.3 validation, so they need to agree with config.
- Git history is full of repeated `feat(atlas): add age recipient` / `chore: update nix-secrets flake lock` commits — a symptom of non-idempotent re-runs that Stage 4.2 should fix.

---

## 4. Installer (Go) — current state

```
cmd/nixos-installer/main.go          builds []Stage{ProviderPreparationStage}, hardcoded host=atlas target=192.168.1.100
internal/pipeline/pipeline.go        sorts by Index(), skips Index <= state.StageIndex, runs stages
internal/stage/stage.go              interface { Index() int; Name() string; Run(ctx,*State) error; Rollback(ctx,*State) error }
internal/stages/01_provider_preparation.go   pings google.com via local executor
internal/state/state.go              HostName, Target, Stage, StageIndex, HostRecipient, SecretsCommit, Converged
internal/command/{command.go,result.go}       Command{Name,Args}, Result{Output,ExitCode,Err}
internal/command/executor/{executor.go,local.go}  Executor.Run(ctx,cmd) via os/exec, stdout+stderr merged into a buffer
internal/command/adapter/ping.go     ping -c 1 <host>
internal/log/                        live task-tree logger, see docs/LOG.md (replaced internal/logger)
internal/cli/bootstrap.go            empty
```

Bugs / gaps found:
- `pipeline.Run`: the partition loop appends **every** stage to `unprocessedStages` (missing `else`), so the "all processed" check never fires; the run loop iterates `p.stages` instead of the unprocessed list; `StageIndex` is never advanced; state is never persisted; `Rollback` is never called.
- Stage 01 pings `google.com` instead of `state.Target`, and passes `context.Background()` instead of `ctx`.
- `main` swallows the pipeline error (empty `if`), never sets an exit code; `--verbose` flag is commented out; host/target hardcoded.
- Local executor has no stdin, env, workdir, streaming output, or TTY passthrough — all needed (secrets via stdin, `nixos-anywhere` progress, interactive vim).
- `ping` flags differ between macOS (the provider, workhorse) and Linux: `-W` means different things. Prefer a TCP dial to :22 in Go over shelling out to ping.
- Stage numbering is `int` (1) while the Pipeline uses `0.1 … 4.2`.

---

## 5. Legacy bash → Pipeline mapping

| Pipeline stage | Runs on | Legacy `install-host.sh` equivalent | Port notes |
|---|---|---|---|
| 0.1 Provider preparation | provider | `eval_spec`, preflight: nix-secrets present, disk not `REPLACE_ME`, ping, `ssh root@ip`, remote `nix`/`lsblk`/internet check, `test -b $DEVICE`, show `lsblk`, type "install", LUKS prompt → `/tmp/disko-password` | Keep the typed confirmation + lsblk display. Stream the LUKS pass via stdin, never argv. Use a TCP dial instead of ping. |
| 0.2 Base install | provider → target | `nix run github:nix-community/nixos-anywhere -- [--generate-hardware-config …] --build-on-remote --flake .#<host>-bootstrap --target-host root@ip`; commit hardware config; skip if `/run/current-system` exists | Then wait for SSH (`sshWaitTimeout`, poll 5s), TOFU then pin ed25519 host key → state. |
| 1.1 Host identity | target | `cat /etc/ssh/ssh_host_ed25519_key.pub \| ssh-to-age` (run on provider) | Can be done in Go with `github.com/Mic92/ssh-to-age` (library) — no tool needed. Store recipient + fingerprint in state. |
| 1.2 Readiness | target | partial (internet ping) | Add `git ls-remote`-style GitHub reachability + `git sops vim nix` presence. Marker `awaiting-secrets-phase`. |
| 2.0 Auth bootstrap | target | — (relied on operator's SSH agent on provider) | New. See §6.3. |
| 2.1 Sync repo | target | — (used local `../nix-secrets`) | New: clone/fetch/reset on target. |
| 2.2 Recipients / .sops.yaml | target | python insert of `&host_<h>` + append rule; conflict check when anchor exists with another key | Structural YAML edit, idempotent, also host-users rules. |
| 2.3 Template authoring | target | `cp templates/host.yaml` + `sops --encrypt --in-place` | New: per-user templates, interactive vim over `ssh -t`, schema validation. |
| 2.4 Encrypt/commit/push | target | `sops updatekeys -y hosts/<h>.yaml shared.yaml`, `git add -A; commit; push` | Commit only the listed paths (not `-A`). Verify `sops:` metadata in every secret file. Persist SHA. |
| 3.1 Flake input | provider or target | `nix flake update nix-secrets` + commit `flake.lock` (in config, on provider) | See §6.4. |
| 3.2 Rebuild | target | `nixos-rebuild --flake .#<h> --target-host root@ip --build-host root@ip switch` (from provider) | Pipeline wants it on target. Validation list from legacy Phase 9 is reusable. |
| 3.3 Cleanup | target | `trap rm KNOWN_HOSTS` only | New: wipe temp creds, optional upstream revoke, marker `converged`. |
| 4.1 Policy | provider/target | — | New: no plaintext, rule coverage for every `hosts/*`/`host-users/*`. |
| 4.2 Recovery | — | `--enroll-sops`, `--deploy-remote` modes | Becomes "resume from persisted state". |

Legacy Phase 9 validation checks worth keeping: `systemctl --failed`, `sops-nix` active, `/run/secrets-for-users/passwords/<u>` readable, `~<u>/.ssh/id_ed25519` mode 600, `home-manager-<u>.service` not failed, `nixos-version`.

---

## 6. Gaps between the Pipeline and the current repos (decisions needed)

### 6.1 Secret path model changes — DECIDED: migrate to pipeline paths
Pipeline canonical paths are `hosts/<host>.yaml` and `host-users/<host>-<user>.yaml`. nix-secrets is being migrated to them. Config still reads `users/<user>.yaml` (shared across hosts), so `hosts/common/accounts/secrets.nix` and `home/common/core/sops.nix` in config have to follow, and `users/jkragt.yaml` moves to `host-users/<host>-jkragt.yaml` for each host. The templates (`templates/host.yaml`, `templates/user.yaml`) should match the keys config reads, because the installer uses them as the schema in Stage 2.3.

### 6.2 Who decrypts `host-users/*` — and the operator key
Today `users/<u>.yaml` is encrypted only to `&user_<u>`, so the legacy flow **copies the operator's age identity onto every device** (README calls this out as a security consequence). With per-host-user files:
- Recipients = `&host_<host>` (so the target can decrypt unattended) + `&user_<u>` / operator (so you can edit from your laptop).
- Then the target needs no copy of the operator key: user secrets can be decrypted by system sops-nix with `owner = <u>` and a path in the home dir, instead of Home Manager sops with `~/.config/sops/age/keys.txt`. That makes `provisionUserAgeKey`, the pending-key activation script and Phase 7.5 of the bash script obsolete.
- Decision: adopt this (recommended — it is what the Pipeline implies) or keep HM-sops with a delivered key.

### 6.3 Git auth on the target vs. the `git+ssh` flake input
- `nix-secrets` is fetched by Nix as `git+ssh://`. A GitHub App token or fine-grained PAT is HTTPS-only, so Phase 3 evaluation on the target would not use it unless the URL is rewritten (`--override-input nix-secrets git+https://…` or git `insteadOf` in a temp `GIT_CONFIG_GLOBAL`) — whether Nix's git fetcher honours `insteadOf` in your Nix version needs verifying.
- A **deploy key** (SSH, write access) fits the existing URL directly via a temp `GIT_SSH_COMMAND` / ssh config, but deploy keys are per-repo. If the target also has to clone/push `nixos-config` (private? Stage 3.1), it needs a second key — or use a GitHub App token covering both repos.
- Alternative that avoids target write-access to config entirely: see 6.4.

### 6.4 Where the flake.lock update happens
Committing `flake.lock` on the target needs push access to `nixos-config` too. Options:
1. Target: `nixos-rebuild switch --flake <config>#<host> --override-input nix-secrets 'git+ssh://…?ref=main&rev=<SHA>'` (no lock commit on target), and the provider runs `nix flake update nix-secrets` + commit + push afterwards.
2. Provider updates and pushes the lock after Stage 2.4, target pulls config at that commit.
3. Target does it all (needs auth for both repos).

### 6.5 Recipients for users
"Ensure user recipient entries exist for all host users" — source of truth is undecided. Existing options: the `&user_<u>` anchors in `.sops.yaml` (only `jkragt` exists), or derive with ssh-to-age from `config/hosts/common/accounts/<u>/keys/*.pub` (jkragt has `workhorse.pub` + a `PLACEHOLDER.pub`; `media` has no keys dir). `broadway` would fail today because `media` has no recipient. Fail-closed with a clear message is the safe default.

### 6.6 Re-install = new host key
A reinstall generates a new host key, so `&host_<host>` changes and the old `hosts/<host>.yaml` can no longer be decrypted on the target. The bash script dies ("remove manually"). The Go installer needs a policy: replace the anchor and re-author from template on the target, or have the operator rekey from the provider (who holds a user key that is also a recipient). Alternative: pre-generate the host key on the provider and inject it with `nixos-anywhere --extra-files` so the recipient is known (and stable) before install.

### 6.7 Editing on the target
- Encrypting needs only public recipients — fine on the target.
- Re-editing an already-encrypted host or host-user file on the target needs a private key: the host SSH key works if the host is a recipient (sops supports SSH keys for age decryption via `SOPS_AGE_SSH_PRIVATE_KEY_FILE` in recent versions — verify against the sops version in nixpkgs).
- `shared.yaml` is encrypted to the user only, so the target can **not** `updatekeys` it (the bash script did, from the provider). Either keep shared-secret rekeying as a provider-side step or drop it from the target flow.

### 6.8 Bootstrap config additions
Add to the bootstrap module (or `installSpec`): `git`, `sops`, `age`, `ssh-to-age`, `vim`, and whatever runtime the installer needs on the target (if the Go binary itself runs there, either `nix run` it from a flake or `scp` a static `GOOS=linux` build).

### 6.9 Housekeeping spotted
- `hosts/nixos/atlas/default.nix` sets `users.users.root.initialPassword = "test"` in the full config — remove.
- `broadway/disko.nix` device is still `REPLACE_ME`.
- The installer folder is not a git repo yet.

---

## 7. Suggested direction for the Go code (fits what's already there)

- **State**: persist `State` as JSON per host (e.g. `~/.local/state/nixos-installer/<host>.json`), save after every stage. Add: `Users []string`, `PrimaryUser`, `HostKeyFingerprint`, `HostRecipient`, `UserRecipients map[string]string`, `SecretsCommit`, `ConfigRev`, `Marker` (`awaiting-secrets-phase`, `converged`), `CompletedStages []string`.
- **Stage IDs**: use string IDs matching the Pipeline (`"0.1"`, `"2.3"`…) or keep `int` but map 1:1 and document it; resume = first stage not in `CompletedStages`.
- **Executors**: `Local` + `SSH` (target) sharing one interface; options for stdin (`io.Reader`, for secrets), env, dir, streamed output to the logger, and an interactive mode (`ssh -t`, attach os.Stdin/Stdout, stop the spinner first) for vim.
- **Adapters** to add: `nix eval --json`, `nixos-anywhere`, `ssh`/`ssh-keyscan`, `git`, `sops`, `nixos-rebuild`. Do ssh→age conversion and `.sops.yaml` editing in-process.
- **CLI**: `install --host --target` (full) and `secrets --host --target` (Phase 2–3 resume, as in Pipeline 4.2); flags for `--verbose`, `--config`, `--state-dir`.
- **Secrets hygiene**: never put secrets in argv or logs (bash script already follows this — keep it), always `umask 077` on target, cleanup in a deferred step even on failure.
