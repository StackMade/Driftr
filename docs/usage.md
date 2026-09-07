# CLI Usage

All commands support the `-v` / `--verbose` flag for detailed output.

## driftr install

Download and install a tool version, or everything the current project pins.

```bash
# Install every tool this project pins
driftr install

# Install latest Node.js 22.x
driftr install node@22

# Install a specific version
driftr install node@22.14.0

# Install pnpm and yarn
driftr install pnpm@9
driftr install yarn@1

# Install the latest version of a tool
driftr install node@latest

# Install the newest active LTS release (node only — pnpm/yarn have no LTS concept)
driftr install node@lts

# Install a named LTS line by codename (matched against the release index)
driftr install node@lts/jod

# Verbose output (shows download URL, checksum verification)
driftr install node@22 -v
```

**Install pipelines by tool:**

| Tool    | Source                                 | Verification                  |
|---------|----------------------------------------|-------------------------------|
| Node.js | nodejs.org archives                    | SHA256 against SHASUMS256.txt |
| pnpm    | npm registry tarball                   | SHA-512 SRI integrity         |
| yarn    | npm registry tarball                   | SHA-512 SRI integrity         |

**Installing what a project pins:**

With no argument, `driftr install` reads the pins for the current directory and
installs each one. It reads the same sources resolution reads, walking up
through parent directories: `.driftr.toml`, the `driftr` key in `package.json`,
the standard `packageManager` field (pnpm and yarn), `.nvmrc`, `.node-version`,
and `engines.node`. The first source that names a tool wins, and each tool is
looked up separately, so a repo can pin Node.js in `.nvmrc` and pnpm in
`packageManager`.

An `engines.node` range does not name one version, so Driftr installs the major
version at the range's lower bound, which is the oldest release the project
accepts. If the range has no lower bound, Driftr installs the newest LTS.

The output names the source each version came from:

```
Installing node@22.14.0 (from .nvmrc)...
✓ Installed node 22.14.0
Installing pnpm@9.15.0 (from package.json (packageManager))...
✓ Installed pnpm 9.15.0
```

If one tool fails, the rest still install and the failures are reported
together at the end. If nothing is pinned, the command says so instead of
exiting quietly.

**Notes:**
- Reinstalling an already-installed version is a no-op
- Downloaded archives and binaries are cached in `~/.driftr/cache/`
- If checksum verification fails, the cached archive is deleted automatically
- If extraction fails, the partial installation is cleaned up

## driftr uninstall

Remove a previously installed tool version.

```bash
driftr uninstall node@22.14.0
driftr uninstall pnpm@9.15.0
driftr uninstall yarn@1.22.22
```

Removes the version directory from `~/.driftr/tools/<tool>/<version>/`.

**Notes:**

- If the version is the current global default, a warning is printed
- If the version is pinned in a `.driftr.toml` or `package.json` in the current directory or any parent directory, a warning naming that file is printed
- Cached archives in `~/.driftr/cache/` are not removed (they will be reused if you reinstall)

## driftr prune

Remove installed versions that nothing references.

```bash
driftr prune --dry-run     # show the plan, delete nothing
driftr prune               # show the plan, then ask before deleting
driftr prune --tool node -y
```

A version counts as referenced when it is the global default for its tool, or when the project in the current directory pins it. Pins are resolved the same way the shims resolve them, so a partial pin such as `24`, an `lts/jod` alias, or an `engines.node` range protects the installed version it would actually select. Everything else is listed with its size and removed after you confirm.

**Scope:** prune sees the global default and the pins that apply to the current directory. It does not scan the machine for other checkouts, so a version that only some other project pins looks unreferenced from here and will be offered for removal. Run it from the project you care about, or start with `--dry-run`.

**Flags:**

- `--dry-run` prints the plan and exits without touching anything
- `-y`, `--yes` skips the confirmation prompt
- `--tool <name>` restricts the run to one tool

**Notes:**

- If one directory cannot be removed, the rest still are, and the failures are reported at the end with an `rm -rf` command for manual cleanup
- Cached archives in `~/.driftr/cache/` are not touched, use `driftr cache clean` for those

## driftr default

Set the global default version for a tool.

```bash
driftr default node@22.14.0
driftr default pnpm@9.15.0
driftr default yarn@1.22.22
```

The global default is used whenever you run a tool outside a project with a pinned version.

**Requirements:**
- The version must already be installed

## driftr pin

Pin a tool version to the current project.

```bash
cd my-project
driftr pin node@22.14.0
driftr pin pnpm@9.15.0
```

On first use, Driftr prompts you to choose a storage format:

```
No existing project config found. How should the version be stored?
  1) .driftr.toml (recommended)
  2) package.json (driftr key)
Choose [1/2]:
```

Choosing `.driftr.toml` creates:

```toml
[tools]
node = "22.14.0"
pnpm = "9.15.0"
```

Choosing `package.json` adds a `driftr` key to your existing `package.json`:

```json
{
  "name": "my-project",
  "driftr": {
    "node": "22.14.0"
  }
}
```

Subsequent `driftr pin` commands detect the existing format and reuse it automatically.

**Migrating between formats:**

```bash
# Switch from .driftr.toml to package.json (or vice versa)
driftr pin node@22.14.0 --migrate
```

This writes the version in the other format and removes the old config.

**Requirements:**
- The version must already be installed
- `package.json` format requires an existing `package.json` file (run `npm init` first)
- `package.json` format supports `node`, `pnpm`, and `yarn`

**Behavior:**
- Anyone who clones the project and has Driftr set up will automatically use the pinned version
- The pinned version takes priority over the global default
- Nested directories inherit the pin until another config overrides it
- In non-interactive environments (CI), defaults to `.driftr.toml`

## driftr use

Switch a tool version for the current shell, without touching any config file.

```bash
eval "$(driftr use node@24)"
eval "$(driftr use node@24.1.0)"
eval "$(driftr use pnpm@9)"
eval "$(driftr use --unset node)"
```

The command prints a snippet and nothing else. `eval` is what applies it:

```bash
$ driftr use node@24
export DRIFTR_NODE=24.1.0
```

The snippet sets `DRIFTR_NODE` (or `DRIFTR_PNPM`, `DRIFTR_YARN`), which the resolver reads
before any project config. The override lives in the shell you ran it in and disappears when
you close it. Other shells and other terminal tabs keep resolving normally.

The version must already be installed, and a partial version picks the newest installed
release that matches, same as `driftr pin`.

Syntax follows `$SHELL`, so fish users get `set -gx DRIFTR_NODE 24.1.0`. Pass `--shell` when
the detection is wrong or when you are writing the line into a script:

```bash
driftr use node@24 --shell fish
```

**A shell function, if you type this often:**

```bash
# ~/.zshenv
dr() { eval "$(driftr use "$@")"; }
```

## driftr list

List installed versions for a tool. Defaults to node.

```bash
driftr list          # list node versions
driftr list pnpm     # list pnpm versions
driftr list yarn     # list yarn versions
```

Output example:

```
Installed node versions:
    20.11.0
  * 22.14.0
    24.0.0

  * = global default
```

**Alias:** `driftr ls`

## driftr which

Show which binary Driftr would execute, and why.

```bash
driftr which node
driftr which pnpm
driftr which yarn
```

Output example:

```
Tool:    node
Version: 22.14.0
Binary:  /home/user/.driftr/tools/node/22.14.0/bin/node
Source:  project config
Project: /home/user/my-project
```

**With verbose tracing:**

```bash
driftr which node -v
```

This shows each step of the resolution chain:

```
  [resolve] Starting node version resolution
  [resolve] Step 1: No explicit override
  [resolve] Step 2: DRIFTR_NODE not set
  [resolve] Step 3: Searching for project config from /home/user/my-project
  [resolve]   Checking: /home/user/my-project/.driftr.toml
  [resolve] Resolved: 22.14.0 from project config (/home/user/my-project)
Tool:    node
Version: 22.14.0
Binary:  /home/user/.driftr/tools/node/22.14.0/bin/node
Source:  project config
Project: /home/user/my-project
```

## driftr run

Run a command under a specific Node.js version without changing any persistent settings.

```bash
# Run npm test using Node 24
driftr run --node 24.0.0 -- npm test

# Run a script with a different version
driftr run --node 20.11.0 -- node script.js
```

**Behavior:**
- The global default and project pin are not changed
- The `--` separator is required between flags and the command
- Exit codes are preserved
- `--node` always pins the Node.js runtime. For `yarn`/`pnpm`, which need
  Node.js to execute but have their own independently pinned version, only
  the Node.js they run under is affected — the tool's own version still
  resolves normally from project/global config (e.g. `driftr run --node
  20.11.0 -- yarn` runs your pinned/default yarn under Node 20.11.0)

## driftr setup

Initialize Driftr directories and generate shim scripts.

```bash
driftr setup
```

**What it creates:**
- `~/.driftr/bin/` with shims for `node`, `npm`, `npx`, `pnpm`, `pnpx`, `yarn`
- `~/.driftr/tools/` for installed tool versions
- `~/.driftr/config/` for global settings
- `~/.driftr/cache/` for downloads

Run this once after installing Driftr, and again after upgrading to regenerate shims.

## driftr cache

Manage the download cache.

```bash
# Remove all cached archives to free disk space
driftr cache clean

# Print the cache directory path
driftr cache dir
```

**Notes:**

- `driftr cache clean` removes all files from `~/.driftr/cache/` and reports the freed space
- Installed tool versions are not affected — only cached downloads are removed
- Cached archives are automatically reused by `driftr install` to skip re-downloads

## driftr doctor

Check your Driftr installation for common problems.

```bash
driftr doctor         # run all checks
driftr doctor --fix   # auto-fix PATH configuration issues
```

Runs 9 checks: PATH presence, shell rc file placement, shim existence, shim binary path,
global default set, defaults installed, conflicting managers (nvm/fnm/volta/n), installed
version counts, and pnpm/yarn without node.

The `--fix` flag automatically adds a PATH export to the correct target file (e.g. `.zshenv`
for zsh users). Any stale export in an old rc file is reported as a warning and can be removed
manually. Other issues require manual action per the printed suggestion.

## driftr self-update

Update Driftr to the latest version.

```bash
driftr self-update
```

After a successful update, automatically migrates PATH configuration if it was placed in an
interactive-only file (e.g. `.zshrc`) to the universal target (`.zshenv` for zsh). Prints a
note about any stale entries in old rc files — safe to remove manually.

## Resolution Order

When you run a tool (`node`, `npm`, `npx`, `pnpm`, `pnpx`, or `yarn`), Driftr resolves the version in this order:

| Priority | Source | When |
|----------|--------|------|
| 1 | Explicit `--node` flag | `driftr run --node 24 -- ...` |
| 2 | `DRIFTR_<TOOL>` variable | Set in the shell, usually via `driftr use` |
| 3 | Project `.driftr.toml` | Found in current or parent directory |
| 4 | `package.json` driftr key | Found in current or parent directory |
| 5 | `package.json` `packageManager` field (pnpm/yarn only) | Found in current or parent directory |
| 6 | `.nvmrc` (node only) | Found in current or parent directory; `lts`, `lts/*` and `lts/<codename>` resolve against installed versions |
| 7 | `.node-version` (node only) | Found in current or parent directory; same LTS aliases as `.nvmrc` |
| 8 | `package.json` `engines.node` (node only) | Found in current or parent directory |
| 9 | Global default | Set via `driftr default` |

If no version is configured at any level, Driftr prints an actionable error.

**Tool resolution:**

- `npm` and `npx` resolve via the **node** version (they are bundled with Node.js)
- `pnpm` and `pnpx` resolve via the **pnpm** version (pnpx is a symlink to pnpm)
- `yarn` resolves via the **yarn** version, and also co-resolves **node** because yarn is a JS script that needs `node` to execute

## Typical Workflow

```bash
# One-time setup
driftr setup
echo 'export PATH="$HOME/.driftr/bin:$PATH"' >> ~/.zshenv  # zsh — use ~/.bash_profile for bash
source ~/.zshenv

# Install your toolchain
driftr install node@22
driftr install pnpm@9
driftr install yarn@1

# Set global defaults
driftr default node@22.14.0
driftr default pnpm@9.15.0

# Pin projects
cd project-a && driftr pin node@22.14.0 && driftr pin pnpm@9.15.0
cd project-b && driftr pin node@24.0.0

# Everything just works -- Driftr handles the rest
cd project-a && node -v   # v22.14.0
cd project-a && pnpm -v   # 9.15.0
cd project-b && node -v   # v24.0.0
cd ~         && node -v   # v22.14.0 (global default)
```
