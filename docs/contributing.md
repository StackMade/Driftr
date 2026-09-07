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

**Integration tests (recommended -- runs in Docker, no local side effects):**

```bash
docker build -f Dockerfile.test -t driftr-test .
docker run --rm driftr-test
```

**Unit tests:**

```bash
go test ./...
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
docs/                documentation
test.sh              integration test script
Dockerfile           production image
Dockerfile.test      test runner image
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

### 3. Test in Docker

Always test in Docker before submitting. This ensures your changes work in a clean environment without relying on your local setup.

```bash
docker build -f Dockerfile.test -t driftr-test .
docker run --rm driftr-test
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
4. Add integration tests in `test.sh`
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

## Adding Integration Tests

Tests live in `test.sh` and run inside Docker. Use the existing helper functions:

```bash
# Check that a command exits successfully
check "description" some-command --args

# Check that command output contains expected text
check_output "description" "expected text" some-command --args
```

Group tests under numbered section headers to keep them organized.

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
