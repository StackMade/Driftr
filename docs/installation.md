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
curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | DRIFTR_VERSION=1.4.0 sh
```

Without `DRIFTR_VERSION` the script installs the latest stable release. A pre-release such as
`2.0.0-rc.1` is only installed when pinned this way; `driftr self-update` then follows the newer
pre-releases up to the stable release.

Override the install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | DRIFTR_INSTALL_DIR=/usr/local/bin sh
```

The variable goes after the pipe: it has to be set for `sh`, which runs the script, not for `curl`.

### Using wget

```bash
wget -qO- https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

## Homebrew (macOS and Linux)

```bash
brew tap StackMade/driftr
brew install driftr
```

### Post-install step

Homebrew places the `driftr` binary in `/opt/homebrew/bin/driftr` (Apple Silicon) or `/usr/local/bin/driftr` (Intel). Run setup once before using it:

```bash
driftr setup
```

This creates the shims and data directories and adds `~/.driftr/bin` to `PATH` in the file every invocation of your shell reads (see [PATH Configuration](#path-configuration) for which file). Restart your shell afterwards and run `driftr doctor` to verify.

**Why PATH ordering matters:** Driftr's shims (`node`, `npm`, `npx`, `pnpm`, `pnpx`, `yarn`, `bun`) live in `~/.driftr/bin/`. For version pinning to work, this directory must appear **before** Homebrew paths in `PATH`.

On macOS, `/usr/libexec/path_helper` runs at login shell startup (often together with `brew shellenv` in `~/.zprofile`) and pushes `/opt/homebrew/bin` to the front, which causes Homebrew's system `node` (if installed) to shadow Driftr's shims. `driftr setup` and `driftr doctor --fix` both check for this once `~/.driftr/bin` is on `PATH`: when another install (Homebrew, Volta, …) shadows a shim, they append a PATH precedence guard to your interactive rc file (`~/.zshrc`, `~/.bashrc`, or fish `config.fish`), which is read last, so the shims win. If Homebrew's `node` still wins after you restart the shell, run `driftr doctor --fix`.

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

## Windows

From PowerShell (5.1 or later):

```powershell
irm https://raw.githubusercontent.com/StackMade/driftr/main/install.ps1 | iex
```

The script follows the same steps as `install.sh`: it downloads `driftr_<version>_windows_<arch>.zip`,
checks it against `checksums.txt`, installs `driftr.exe` to `%USERPROFILE%\.driftr\bin`, and runs
`driftr setup`. Open a new terminal afterwards; `driftr doctor` checks the result.

Windows builds start with Driftr 2.0. While the latest stable release is still a 1.x, the script
installs the newest 2.0 pre-release instead and says so.

`DRIFTR_VERSION` and `DRIFTR_INSTALL_DIR` work as they do for `install.sh`. Set them in the same
PowerShell session first:

```powershell
$env:DRIFTR_VERSION = '2.0.0'
irm https://raw.githubusercontent.com/StackMade/driftr/main/install.ps1 | iex
```

### How Windows differs

- **Shims are executables.** `%USERPROFILE%\.driftr\bin` holds `node.exe`, `npm.exe`, `npx.exe` and
  the rest, each a hard link to (or, across volumes, a copy of) `driftr.exe`. driftr picks the tool
  from the name it was started under. A `.cmd` shim would make `cmd.exe` parse every argument a
  second time and ask "Terminate batch job (Y/N)?" on Ctrl-C.
- **PATH lives in the registry.** `driftr setup` puts the shim directory first in your user `Path`
  (`HKCU\Environment`), which cmd, PowerShell and Git Bash all start from. Windows lists the system
  `Path` before the user one, so a Node.js installed for all users still comes first: uninstall it,
  or, as an administrator, move its directory after driftr's. `driftr doctor` reports which tool is
  shadowed and by what.
- **The home directory is `%USERPROFILE%`**, never `%HOME%`, so cmd, PowerShell and Git Bash share
  one `.driftr` directory even when Git Bash sets `HOME` elsewhere.
- **Global npm packages** install into the Node.js version's own directory, as on other platforms.
  npm writes `tsc.cmd` there; driftr gives it a `tsc.exe` shim that runs the script the `.cmd`
  names under that version's `node.exe`.

---

## Building from Source

Driftr is written in Go. You need Go 1.27 or later to build it.

### Prerequisites

- [Go](https://go.dev/dl/) 1.27+
- Git
- macOS, Linux or Windows

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
go install github.com/stackmade/driftr/v2/cmd/driftr@latest
```

## Initial Setup

After installing the binary, run setup to create directories and shim scripts and to put the shims on your `PATH`:

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

`stores/pnpm/` appears later, once `driftr node optimize` points pnpm's shared store there.

## PATH Configuration

The shim directory has to come at the **beginning** of your `PATH` so Driftr's shims take priority over any system-installed Node.js. `driftr setup` writes that line for you, and so do `install.sh` and `driftr doctor --fix`. It goes into the file every invocation of your shell reads, picked from `$SHELL`:

| Shell | File |
|-------|------|
| zsh | `~/.zshenv` (`$ZDOTDIR/.zshenv` when `ZDOTDIR` is set) |
| bash | `~/.bash_profile` |
| fish | `~/.config/fish/conf.d/driftr.fish` (under `$XDG_CONFIG_HOME` when set) |
| anything else | `~/.profile` |

The rest of this section is the manual fallback, for when you'd rather edit the files yourself or setup could not write them.

### Zsh (~/.zshenv)

```bash
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshenv
```

`.zshenv` is sourced by every zsh invocation — interactive shells, scripts, cron, and IDE
terminals — and is the primary location. On its own it is not enough for interactive
shells on macOS, though:

> **macOS note:** Login shells invoke `/usr/libexec/path_helper`, which reorders PATH and pushes
> Homebrew (`/opt/homebrew/bin`) to the front. To guarantee Driftr wins, also add the export to
> `~/.zshrc` (sourced last in interactive sessions). `install.sh` does this on macOS; `driftr setup`
> and `driftr doctor --fix` do it when they find a shim shadowed:
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

On Windows, delete `%USERPROFILE%\.driftr` and remove its `bin` directory from your user `Path`
(Settings, then "Edit environment variables for your account").
