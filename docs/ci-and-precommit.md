# Continuous Integration And Pre-Commit

Use `gox check` as the non-mutating repository gate. It reports both formatting
differences and enabled lint diagnostics, sorts output deterministically, and
does not rewrite source. Exit code 1 means actionable findings; exit codes 2
through 6 and 130 are tool, source, configuration, filesystem, conflict, or
cancellation outcomes rather than ordinary findings.

Gox is still a development identity. Until the final naming audit and public
release, provision one exact reviewed source revision and do not treat the
repository or command path below as a stable installation contract.

## GitHub Actions

The following job checks out the project and a full, reviewed Gox commit into
separate directories. Replace `<full-gox-commit-sha>` with a 40-character
commit ID. Keeping the tool source outside the project checkout prevents
`./...` from selecting Gox itself.

```yaml
name: Gox

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  check:
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    steps:
      - name: Check out project
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          path: project
          persist-credentials: false
      - name: Check out pinned Gox source
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          repository: faustbrian/gox
          ref: <full-gox-commit-sha>
          path: gox-tool
          persist-credentials: false
      - name: Install the recorded Go toolchain
        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e
        with:
          go-version: 1.26.5
          cache: false
      - name: Build pinned Gox
        shell: bash
        working-directory: gox-tool
        env:
          GOTOOLCHAIN: local
        run: |
          set -euo pipefail
          task_root=$(mktemp -d "$RUNNER_TEMP/gox-build.XXXXXX")
          cleanup() {
            trap - EXIT HUP INT TERM
            if [ -d "$task_root/modcache" ] &&
              ! GOMODCACHE="$task_root/modcache" go clean -modcache >/dev/null 2>&1; then
              chmod -R u+w "$task_root/modcache"
            fi
            find "$task_root" -mindepth 1 -delete
            rmdir "$task_root"
          }
          trap cleanup EXIT HUP INT TERM
          GOCACHE="$task_root/cache" GOMODCACHE="$task_root/modcache" \
            GOENV=off GOWORK=off go build -o "$RUNNER_TEMP/gox" ./cmd/gox
      - name: Check formatting and lint
        working-directory: project
        run: "$RUNNER_TEMP/gox check ./..."
```

The workflow intentionally pins action commits, the Go toolchain, and Gox
source. It grants no write permission and invokes no fix mode. If a repository
needs machine diagnostics, replace the final command with:

```sh
"$RUNNER_TEMP/gox" check --reporter=json ./... > gox-report.json
```

Preserve the command's exit status when uploading the report. A JSON document
with `complete: false` is not a successful partial check.

## Versioned Git Hook

Keep the hook in the repository rather than copying unrelated versions into
each developer's `.git/hooks` directory. For example, commit this executable as
`.githooks/pre-commit`:

```sh
#!/bin/sh
set -eu

: "${GOX_BIN:=gox}"
exec "$GOX_BIN" check ./...
```

Each developer enables the repository-owned hook once:

```sh
git config core.hooksPath .githooks
```

Before the public release, set `GOX_BIN` explicitly to the repository's
reviewed pinned build; leaving it unset resolves `gox` from `PATH`. The hook
uses the same non-mutating command as CI and must not use `fmt --write`, `lint
--fix`, suggestion fixes, or unsafe fixes. Developers can run those commands
deliberately before committing and review the resulting source changes.

Gox checks filesystem content, not staged Git blobs. With partially staged
files, the hook therefore evaluates the complete working-tree file. Reconcile
partial staging before treating the hook result as proof of the exact commit,
or have CI check the resulting commit after it is created.

## Scope And Configuration

Run from the project root so `.gox.toml`, `go.mod`, `go.work`, and recursive
patterns resolve consistently. Typed selections must share one project root
and configuration. Separate heterogeneous roots into distinct invocations.

The configured Go source version, build tags, GOOS, GOARCH, cgo selection, and
enabled lint tiers are result inputs. Keep the same configuration in local
hooks and CI rather than relying on ambient target variables. Generated files
remain readable but are not writable; `check` itself never writes them.

During formatter migration, remove competing gofmt, gofumpt, or golines gates
in the same coordinated change. See the
[formatter migration guide](migration-from-go-formatters.md) and
[command reference](command-reference.md) for the complete adoption boundary.
