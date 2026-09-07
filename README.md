<p align="center">
  <img src="logo.png" alt="Driftr" width="200" />
</p>

<h1 align="center">Driftr</h1>

<p align="center">
  <strong>Fast JavaScript toolchain versioning without the friction.</strong>
</p>

<p align="center">
  A lightweight JavaScript toolchain manager built for speed and simplicity.<br>
  The spiritual successor to <a href="https://github.com/volta-cli/volta/issues/2080">Volta</a>, made for developers by developers.
</p>

<p align="center">
  <a href="https://codecov.io/gh/StackMade/Driftr"><img src="https://codecov.io/gh/StackMade/Driftr/branch/main/graph/badge.svg" alt="codecov" /></a>
</p>

---

## Why Driftr?

[Volta is no longer maintained.](https://github.com/volta-cli/volta/issues/2080) If you liked Volta's "pin and forget" model -- where `node`, `pnpm`, and `yarn` just work without manual switching -- Driftr carries that torch forward.

Driftr is a new project. It doesn't have Volta's years of polish or fnm's community size. But it has a clean foundation, an honest design, and an active maintainer who actually uses it. If you're looking for something simple that does the job, give it a try. If it's missing something you need, [open an issue](https://github.com/StackMade/Driftr/issues) -- we're listening.

- **Multi-tool** -- manages Node.js, pnpm, and yarn from a single CLI
- **Shim-based** -- `node`, `npm`, `npx`, `pnpm`, `pnpx`, and `yarn` just work, resolved per-project or globally
- **Fast** -- near-zero overhead via `syscall.Exec` process replacement
- **Minimal** -- 2 external dependencies (cobra + toml), everything else is Go stdlib
- **Deterministic** -- explicit resolution chain: `DRIFTR_<TOOL>` (one shell, via `driftr use`) > project config > `package.json` (`driftr` key, then `packageManager` field for pnpm/yarn) > `.nvmrc` / `.node-version` (node) > `package.json` `engines.node` (node) > global default
- **nvm-compatible pins** -- `.nvmrc` and `.node-version` are read as they are, including the `lts`, `lts/*` and `lts/<codename>` aliases
- **Secure** -- SHA256 and SHA-512 SRI checksum verification on every download
- **Simple** -- a handful of commands cover the entire workflow

## Install

### Homebrew (macOS and Linux)

```bash
brew tap StackMade/driftr
brew install driftr
```

After installing, run `driftr setup` — it generates the shims and configures your shell PATH automatically (writing to a universally-sourced rc file such as `~/.zshenv`). Restart your shell afterwards. See [docs/installation.md](docs/installation.md) for details and why PATH ordering matters on macOS.

### Quick Install (curl)

```bash
curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
```

This downloads the latest release, verifies its checksum, and configures your PATH automatically. See [docs/installation.md](docs/installation.md) for all installation methods.

## Quick Start

```bash
# In a repo that already pins its tools, install what it asks for
driftr install

# Or install tools by name
driftr install node@22
driftr install pnpm@9
driftr install yarn@1

# Set global defaults
driftr default node@22.22.0
driftr default pnpm@9.15.0

# Pin a project (prompts for .driftr.toml or package.json on first use)
cd my-project
driftr pin node@22.22.0
driftr pin pnpm@9.15.0

# Everything just works
node -v   # resolves automatically
pnpm -v   # resolves automatically
```

## Commands

| Command | Description |
|---------|-------------|
| `driftr install [tool[@version]]` | Download and install a tool version (node, pnpm, yarn); with no argument, installs everything the current project pins; a bare tool name installs the latest; `node@lts` installs the newest LTS release, `node@lts/jod` a named LTS line |
| `driftr uninstall <tool@version>` | Remove an installed tool version |
| `driftr default <tool@version>` | Set the global default version for a tool |
| `driftr pin <tool@version>` | Pin a version to the current project (`.driftr.toml` or `package.json`) |
| `driftr use <tool@version>` | Print a shell snippet pinning a version for the current shell: `eval "$(driftr use node@24)"` |
| `driftr list [tool]` | List installed versions (defaults to node) |
| `driftr list --remote [tool]` | Browse available remote versions from nodejs.org / npm registry |
| `driftr which <tool>` | Show which binary would be executed and why |
| `driftr run --node <ver> -- <cmd>` | Run a command under a specific Node.js version |
| `driftr setup` | Initialize Driftr, generate shims, and configure your shell PATH |
| `driftr cache clean` | Remove all cached downloads to free disk space |
| `driftr cache dir` | Print the cache directory path |
| `driftr self-update` | Update Driftr to the latest version |
| `driftr doctor [--fix]` | Check your Driftr installation for common problems |
| `driftr node doctor` | Analyze the project's Node.js / pnpm dependency environment |
| `driftr node optimize [--install]` | Configure pnpm for shared dependency storage (idempotent) |
| `driftr node clean [--yes]` | Remove `node_modules` and prune the shared store (dry-run by default) |
| `driftr node report` | Report `node_modules` size and shared pnpm store size |

All commands support `-v` / `--verbose` for detailed output including resolver tracing and checksum details.

Output is colorized when writing to a terminal. Color is disabled automatically when output is piped/redirected, or when the [`NO_COLOR`](https://no-color.org/) environment variable is set.

### Shared dependency storage (`driftr node`)

Driftr does not replace pnpm/npm/yarn. The `driftr node` commands configure and
maintain pnpm's shared content-addressable store so dependencies are stored once
and reused across every project on the machine, instead of duplicated in each
`node_modules`.

```bash
driftr node doctor      # see current package manager + pnpm store configuration
driftr node optimize    # enable corepack, point pnpm at ~/.driftr/stores/pnpm,
                        # and turn on the global virtual store
driftr node report      # compare project node_modules size vs shared store size
driftr node clean       # dry-run: show what would be removed/pruned
driftr node clean --yes # remove node_modules, reinstall, prune orphaned packages
```

`optimize` is idempotent — already-correct settings are left untouched — and
requires pnpm (run `corepack enable` first if it is missing).

### Remote version listing flags

```bash
driftr list --remote [tool]          # Show available versions (default: latest 30)
driftr list --remote --limit 10      # Limit output to 10 versions
driftr list --remote --limit 0       # Show all versions (can be 500+ for node)
driftr list --remote --pre pnpm      # Include pre-release versions (npm packages only)
```

Installed versions are marked with `●`, the active version with `>`, and the global default with `*`. Node.js LTS releases show their codename (e.g. `LTS: Jod`).

## Shell Completions

```bash
# zsh
echo 'eval "$(driftr completion zsh)"' >> ~/.zshrc

# bash
echo 'eval "$(driftr completion bash)"' >> ~/.bashrc

# fish
driftr completion fish | source
```

## How It Works

```mermaid
flowchart TD
    A["$ node app.js"] --> B["shim (bin/)"]
    B --> C["resolver"]
    C --> C1["1. explicit flag"]
    C --> C1b["2. DRIFTR_NODE env var\n(set by driftr use)"]
    C --> C2["3. .driftr.toml\n(walks up dirs)"]
    C --> C3["4. package.json driftr key\n(walks up dirs)"]
    C --> C4["5. .nvmrc (node only)\n(walks up dirs)"]
    C --> C5["6. .node-version (node only)\n(walks up dirs)"]
    C --> C6["7. package.json engines.node\n(node only, newest installed match)"]
    C --> C7["8. global config.toml"]
    C --> C3b["4b. package.json packageManager field\n(pnpm/yarn only, walks up dirs)"]
    C1 & C1b & C2 & C3 & C3b & C4 & C5 & C6 & C7 --> D["syscall.Exec\nreplaces process with real node"]
```

Shims in `~/.driftr/bin/` intercept calls to `node`, `npm`, `npx`, `pnpm`, `pnpx`, and `yarn`. The resolver determines the correct version, and `syscall.Exec` replaces the process with the real binary. Standalone tools (node, pnpm) are exec'd directly. Tools that need Node.js (yarn) are exec'd as `node <tool-script>`.

## Documentation

| Document | Description |
|----------|-------------|
| [Installation](docs/installation.md) | Detailed install guide for macOS and Linux |
| [Usage](docs/usage.md) | Full CLI reference with examples |
| [Configuration](docs/configuration.md) | Global and project config format |
| [Architecture](docs/architecture.md) | Internal design and module overview |
| [Contributing](docs/contributing.md) | How to contribute to the project |

## Project Layout

```
~/.driftr/
  bin/              shims (node, npm, npx, pnpm, pnpx, yarn)
  tools/
    node/           installed Node.js versions
    pnpm/           installed pnpm versions
    yarn/           installed yarn versions
  config/
    config.toml     global default settings
  cache/            downloaded archives + binaries
```

## How Driftr Compares

| | **Driftr** | **nvm** | **Volta** | **fnm** | **mise** |
|---|---|---|---|---|---|
| Language | Go | Shell | Rust | Rust | Rust |
| Mechanism | Shims | Shell function | Shims | PATH manipulation | PATH manipulation |
| Shell startup cost | ~1ms | 200-500ms | ~1ms | ~1ms | ~5ms |
| External dependencies | **2** | 0 (shell) | ~36 crates | 24 crates | 113 crates |
| macOS / Linux | Yes | Yes | Yes | Yes | Yes |
| Windows | No | No | Yes (rough) | Yes | Very basic |
| Manages npm/pnpm/yarn | **Yes** | No | Partial | No | Yes |
| Maintained | Yes | Yes | **No** | Yes | Yes |
| Self-update | `driftr self-update` | `nvm` script | No | No | `mise self-update` |

**When to choose Driftr**: You want a fast, minimal, shim-based manager for Node.js, pnpm, and yarn with a Volta-like experience -- pin versions to projects, and tools just work. You value simplicity and a small dependency footprint.

**When to choose something else**: If you need Windows support, fnm is your best bet. If you want one tool for Node + Python + Ruby + everything else, mise is the polyglot option. If nvm already works for you and startup time doesn't bother you, there's no reason to switch.

## Requirements

- macOS or Linux
- `curl` or `wget` (for the install script)
- Internet access (to download releases from nodejs.org, GitHub, and the npm registry)
- Go 1.26+ (only if building from source)

## License

MIT

## Contributing

See [docs/contributing.md](docs/contributing.md) for guidelines on how to contribute.
