Bring `docs/CONTEXT.md` in line with `docs/ROADMAP.md` and check three assumptions the roadmap depends on.

1. Run `go build ./... && go vet ./... && go test -race ./...` (Go at `~/sdk/go1.27.1/bin/go`) and report the result.
2. Update `docs/CONTEXT.md`. §1, §4 and §7 are current; leave them. Rewrite §3 to the nix-secrets layout as it is now (look at `~/src/nix-secrets`), mark §6.2–6.7 as decided with the outcome from the roadmap's "Decisions" table, and bring the "Decisions so far" block, the snapshot date and the "Predecessor" note up to date.
3. On atlas (already installed), with `ssh -A root@<atlas-ip>`, check the following and write the outcome into a new "Verified assumptions" section of `docs/ROADMAP.md`:
   - with the forwarded agent as root: `ssh-add -l`, `git ls-remote git@github.com:JulianKragt/nix-secrets.git`, and `nix flake metadata 'git+ssh://git@github.com/JulianKragt/nix-secrets.git'` all work;
   - `SOPS_AGE_SSH_PRIVATE_KEY_FILE=/etc/ssh/ssh_host_ed25519_key sops -d hosts/atlas.yaml` works with the sops version in nixpkgs; if not, whether `SOPS_AGE_KEY="$(ssh-to-age -private-key -i /etc/ssh/ssh_host_ed25519_key)"` does;
   - whether git over ssh to GitHub runs without a host-key prompt.

Ask me for the atlas IP. Do not change anything on atlas.
