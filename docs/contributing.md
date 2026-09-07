# Contributing to Driftr

Thank you for your interest in contributing to Driftr! This guide will help you get started.

## Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.26 or later
- [Docker](https://docs.docker.com/get-docker/) (for running integration tests)
- Git

### Fork and Clone

```bash
git clone https://github.com/<your-username>/driftr.git
cd driftr
```

### Build

```bash
go build -o driftr ./cmd/driftr/
```

### Run Tests

```bash
go test ./...
```

That covers the unit tests and the e2e suite in `e2e/`. The e2e scripts run the real
driftr binary, but each one gets its own `$HOME` and talks to a fake nodejs.org, npm
registry and bun release feed started in-process. Running them locally leaves `~/.driftr`
alone and needs no network.

To run a single e2e script and see every command it executes:

```bash
go test ./e2e -run 'TestDriftr/install-node' -v
```

### Coverage

The e2e suite runs driftr as a subprocess, so a plain `-coverprofile` credits it with
nothing. Setting `DRIFTR_E2E_COVERDIR` makes `e2e/e2e_test.go` build the binary with `go
build -cover` instead and point every script's `GOCOVERDIR` at that directory; `go tool
covdata` then turns the raw profiles into a text profile that appends to the one `go test`
writes. The variable is driftr's own because `go test -coverprofile` overwrites
`GOCOVERDIR` in the test binary's environment:

```bash
mkdir -p /tmp/e2e-coverdata
DRIFTR_E2E_COVERDIR=/tmp/e2e-coverdata go test -race -count=1 -coverprofile=unit.cov ./...
go tool covdata textfmt -i=/tmp/e2e-coverdata -o e2e.cov
cat unit.cov > coverage.txt
tail -n +2 e2e.cov >> coverage.txt
```

`-count=1` matters: a cached test result replays the recorded profile without running
`TestMain`, so the coverdata directory stays empty and the e2e half of the coverage
disappears without any error.

That is exactly what the `test` job in CI runs. Without `DRIFTR_E2E_COVERDIR` nothing
changes and no directories are left behind. Two paths stay invisible either way: a driftr run that
ends in `os.Exit` never flushes its counters, and neither does the shim, which hands the
process over with `syscall.Exec`.

### Linting

`golangci-lint` runs in CI with the config in `.golangci.yml`. Two of its checks catch
things `go vet` does not and are easy to trip over:

- `misspell` enforces US spelling in Go source, comments included. Write "behavior", not
  "behaviour"; "recognized", not "recognised".
- `errcheck` wants every returned error handled. For a write to a `bytes.Buffer`, which
  cannot fail, discard it explicitly with `_ =` and say why in a comment.

The pinned version is a minor (`v2.11`), so a patch release can start reporting something
that passed yesterday. If `Lint` fails on code you did not touch, that is usually why.

### Fuzzing

The hand-written parsers have `go test -fuzz` targets: `FuzzParse` and `FuzzParseRange` in
`internal/version`, and `FuzzParseVersionFile`, `FuzzLoadPackageJSON` and
`FuzzPatchTopLevelKey` in `internal/config`. They run as ordinary tests over their seed
corpus during `go test ./...`, and the nightly workflow gives each one two minutes of real
fuzzing, uploading any crasher it finds as an artifact.

To fuzz one target locally:

```bash
go test ./internal/version/ -run '^$' -fuzz FuzzParse -fuzztime 60s
```

A crasher lands in `internal/<pkg>/testdata/fuzz/<Target>/`. Commit it — it becomes a
regression seed for every later run.

The PATH bootstrap suite is separate, because it needs a real login shell:

```bash
docker build -f Dockerfile.path-e2e -t driftr-path-e2e .
docker run --rm driftr-path-e2e
```

## Project Structure

```
cmd/driftr/          entry point
internal/
  cli/               CLI commands (cobra)
  config/            TOML + JSON config management
  installer/         tool installers (node, pnpm, yarn, bun) + npm registry client
  resolver/          generic version resolution chain
  shim/              shim script generation (node, npm, npx, pnpm, pnpx, yarn, bun)
  process/           process execution (syscall.Exec)
  platform/          OS/architecture abstraction, tool binary map
  version/           semver parsing with tool@ prefix support
  updater/           self-update mechanism
e2e/                 testscript e2e suite (testdata/script/*.txtar)
test/fixture/        fake nodejs.org, npm registry and bun releases
docs/                documentation
test_path_e2e.sh     per-shell PATH bootstrap suite
Dockerfile           production image
Dockerfile.path-e2e  per-shell test runner image
```

See [architecture.md](architecture.md) for a detailed explanation of how the modules interact.

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/your-feature-name
```

Use descriptive branch names:
- `feature/nvmrc-support`
- `fix/checksum-timeout`
- `docs/improve-install-guide`

### 2. Make Your Changes

- Keep changes focused -- one feature or fix per branch
- Follow the existing code style
- Add or update tests for your changes

### 3. Test

```bash
go test -race ./...
```

If you touched anything about PATH setup or shell rc files, run the per-shell suite too:

```bash
docker build -f Dockerfile.path-e2e -t driftr-path-e2e .
docker run --rm driftr-path-e2e
```

### 4. Commit

Write clear commit messages:

```
Add SHA256 checksum verification for downloads

Fetch SHASUMS256.txt from nodejs.org and verify the archive
hash before extraction. Delete cached archive on mismatch.
```

- First line: imperative, under 72 characters
- Blank line, then explanation of what and why (not how)

### 5. Submit a Pull Request

- Push your branch and open a PR against `main`
- Fill in the PR template
- Link any related issues
- Ensure Docker tests pass

## Code Guidelines

### General Principles

- **Keep it simple.** Driftr is intentionally narrow in scope. The right solution is usually the simplest one.
- **No premature abstraction.** Don't add interfaces, factories, or generics until there are at least two concrete use cases.
- **Actionable errors.** Error messages should tell the user what went wrong and how to fix it. Prefer `Node 24.1.0 is not installed. Run driftr install node@24.1.0` over `version not found`.
- **Minimal dependencies.** Use the standard library when possible. New dependencies need a strong justification.

### Go-Specific

- Use `gofmt` (enforced automatically by most editors)
- Use `golint` or `golangci-lint` for static analysis
- Exported functions need doc comments
- Keep packages focused: one responsibility per package
- Error wrapping: use `fmt.Errorf("context: %w", err)` to preserve error chains

### Security

- Sanitize archive paths during extraction (path traversal prevention)
- Verify checksums on all downloaded artifacts
- Never execute untrusted content during installation
- Use HTTPS for all network requests

## Adding a New Command

1. Create `internal/cli/yourcommand.go`
2. Implement the command using cobra
3. Register it in `internal/cli/root.go` via `root.AddCommand(newYourCmd())`
4. Add e2e coverage in `e2e/testdata/script/`
5. Document it in `docs/usage.md`

Example skeleton:

```go
package cli

import (
    "fmt"
    "github.com/spf13/cobra"
)

func newYourCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "yourcommand <args>",
        Short: "Brief description",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            // implementation
            return nil
        },
    }
}
```

## Adding E2E Tests

E2E tests are [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
files under `e2e/testdata/script/`. Each one is a script followed by any files it needs,
in txtar format:

```
# What this script covers.
exec driftr setup
exec driftr install node@22 -v
stdout 'Checksum verified OK'

# A leading ! requires the command to fail.
! exec driftr install unknown@1.0
stderr 'unknown tool'

exec node -v
stdout 'v'$NODE_VERSION

-- project/.driftr.toml --
[tools]
node = "22.99.0"
```

Every script gets a fresh `$HOME` with `$HOME/.driftr/bin` first on `PATH`, plus
`$NODE_VERSION`, `$PNPM_VERSION`, `$YARN_VERSION` and `$BUN_VERSION` holding the versions
the fixture serves. Assert on those variables instead of writing the numbers out.

Two things catch people out:

- `stdout` and `stderr` take a regular expression, so escape dots: `stdout '1\.2\.99'`.
- An error path needs `!` in front of the command. Without it, a command that prints a
  complaint and still exits 0 will pass.

Adding a version or a package to the fixture means editing `test/fixture/fixture.go`.

## Areas for Contribution

### Good First Issues

- Improve error messages with suggestions
- Add color output for `driftr list`
- Add `driftr list --all` to show versions for all tools at once

### Medium Complexity

- Checksum verification for pnpm standalone binaries

### Larger Features

- Windows support (`.cmd` shims, `.zip` extraction)
- yarn berry (3+/4+) support

## Release Process (maintainers)

Releases are fully automated — no version bump in source; build metadata is
injected via ldflags.

```bash
git checkout main && git pull
git tag vX.Y.Z
git push origin vX.Y.Z
```

The tag push triggers `.github/workflows/release.yml`, which:

1. Runs the full test suite (a failing suite blocks the release)
2. Runs GoReleaser: builds linux/darwin × amd64/arm64 archives, generates
   `checksums.txt`, signs it with keyless cosign, publishes the GitHub
   release, and updates the Homebrew tap formula
3. Attests build provenance for all artifacts (verify with
   `gh attestation verify`)

Nightly pre-releases build automatically from main at 02:00 UTC when new
commits exist.

## Reporting Bugs

When reporting a bug, include:

1. Driftr version (or commit hash)
2. Operating system and architecture
3. Steps to reproduce
4. Expected vs actual behavior
5. Verbose output (`-v` flag)

## Questions?

Open a discussion or issue on GitHub. We're happy to help you get started.
