# Installation

## Quick Install (recommended)

Run the installer script to download the latest release, verify its checksum, and set up your PATH:

```bash
curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

The script:

1. Detects your OS and architecture
2. Downloads the latest binary from GitHub Releases
3. Verifies the SHA256 checksum
4. Installs to `~/.driftr/bin/`
5. Runs `driftr setup` to create directories and shims
6. Adds `~/.driftr/bin` to your PATH

### Options

Pin a specific version:

```bash
DRIFTR_VERSION=0.1.0 curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

Override the install directory:

```bash
DRIFTR_INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

### Using wget

```bash
wget -qO- https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

## Homebrew (macOS and Linux)

```bash
brew tap StackMade/driftr
brew install driftr
```

### Required post-install steps

Homebrew places the `driftr` binary in `/opt/homebrew/bin/driftr` (Apple Silicon) or `/usr/local/bin/driftr` (Intel). Two additional steps are required before Driftr works:

**Step 1 — Create shims and data directories:**

```bash
driftr setup
```

**Step 2 — Configure PATH ordering:**

Driftr's shims (`node`, `npm`, `pnpm`, `yarn`) live in `~/.driftr/bin/`. For version pinning to work, this directory must appear **before** Homebrew paths in `PATH`.

On macOS, `/usr/libexec/path_helper` runs at login shell startup (often together with `brew shellenv` in `~/.zprofile`) and pushes `/opt/homebrew/bin` to the front, which causes Homebrew's system `node` (if installed) to shadow Driftr's shims. To fix this, add the PATH export to a file sourced *after* `path_helper`:

**Zsh** — add to both `~/.zshenv` (covers scripts, cron, IDE terminals) and `~/.zshrc` (sourced last in interactive sessions, so the shim dir wins over `path_helper`):

```bash
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshenv
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshrc
```

**Bash** — add to `~/.bash_profile`:

```bash
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.bash_profile
```

Or let Driftr do it for you: `driftr setup` and `driftr doctor --fix` both configure the universal file *and*, when they detect that another install (Homebrew, Volta, …) is shadowing the shims, append a PATH precedence guard to your interactive rc (`~/.zshrc`, `~/.bashrc`, or fish `config.fish`) so the shims win. Run `driftr doctor` to verify your setup.

### Upgrading

```bash
brew upgrade driftr
```

### Uninstalling via Homebrew

```bash
brew uninstall driftr
rm -rf ~/.driftr
```

---

## Building from Source

Driftr is written in Go. You need Go 1.26 or later to build it.

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- Git
- macOS or Linux (Windows support is planned)

### Clone and Build

```bash
git clone https://github.com/StackMade/driftr.git
cd driftr
go build -o driftr ./cmd/driftr/
```

### Install the Binary

Move the binary to a directory in your `PATH`:

```bash
sudo mv driftr /usr/local/bin/
```

Or install it directly with Go:

```bash
go install github.com/stackmade/driftr/cmd/driftr@latest
```

## Initial Setup

After installing the binary, run setup to create directories and shim scripts:

```bash
driftr setup
```

This creates the following structure:

```
~/.driftr/
  bin/           shim scripts (node, npm, npx, pnpm, pnpx, yarn, bun)
  tools/         installed tool versions
  config/        global configuration
  cache/         downloaded archives + binaries
```

## PATH Configuration

Add the shim directory to the **beginning** of your `PATH` so Driftr's shims take priority over any system-installed Node.js.

### Zsh (~/.zshenv)

```bash
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshenv
```

`.zshenv` is sourced by every zsh invocation — interactive shells, scripts, cron, and IDE
terminals — and is the primary location. On its own it is not enough for interactive
shells on macOS, though:

> **macOS note:** Login shells invoke `/usr/libexec/path_helper`, which reorders PATH and pushes
> Homebrew (`/opt/homebrew/bin`) to the front. To guarantee Driftr wins, also add the export to
> `~/.zshrc` (sourced last in interactive sessions). This matches what `install.sh` and
> `driftr doctor --fix` configure automatically:
>
> ```bash
> echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshrc
> ```
>
> See the [Homebrew section](#homebrew-macos-and-linux) above for a detailed explanation.

### Bash (~/.bash_profile)

```bash
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.bash_profile
```

### Fish (~/.config/fish/conf.d/driftr.fish)

```bash
echo 'set -gx PATH $HOME/.driftr/bin $PATH' >> ~/.config/fish/conf.d/driftr.fish
```

Or let `driftr doctor --fix` handle it automatically.

Then reload your shell:

```bash
source ~/.zshenv   # zsh
# or source ~/.bash_profile for bash
```

**Tip:** Run `driftr doctor` to verify PATH is configured correctly. `driftr doctor --fix`
adds a PATH export to the correct target file and regenerates broken shims. Stale entries in
old rc files are flagged but not removed, so clean those up by hand.

### Verify

```bash
which node
# Should output: /Users/<you>/.driftr/bin/node

driftr --help
# Should show the Driftr help menu
```

## Docker

Driftr can also be tested in a Docker container without affecting your local environment.

### Build the image

```bash
docker build -t driftr .
```

### Run commands

```bash
docker run --rm driftr install node@22
docker run --rm driftr list
```

### Run the test suite

```bash
go test ./...
```

## Uninstalling

To remove Driftr:

1. Remove the binary:
   ```bash
   rm /usr/local/bin/driftr
   ```

2. Remove Driftr data:
   ```bash
   rm -rf ~/.driftr
   ```

3. Remove the `PATH` entry from your shell profile.
