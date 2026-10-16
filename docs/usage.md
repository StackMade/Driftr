# CLI Usage

All commands support the `-v` / `--verbose` flag for detailed output. `driftr --version` prints
the version, commit and build date.

## driftr install

Download and install a tool version, or everything the current project pins.

```bash
# Install every tool this project pins
driftr install

# Install latest Node.js 22.x
driftr install node@22

# Install a specific version
driftr install node@22.14.0

# Install package managers
driftr install npm@10
driftr install pnpm@9
driftr install yarn@1
driftr install bun@1

# Install the latest version of a tool
driftr install node@latest

# Install the newest active LTS release (node only, the other tools have no LTS concept)
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
| npm     | npm registry tarball                   | SHA-512 SRI integrity         |
| pnpm    | npm registry tarball                   | SHA-512 SRI integrity         |
| yarn    | npm registry tarball                   | SHA-512 SRI integrity         |
| bun     | GitHub release zip                     | SHA256 against SHASUMS256.txt |

**Installing what a project pins:**

With no argument, `driftr install` reads the pins for the current directory and
installs each one. It reads the same sources resolution reads, walking up
through parent directories: `.driftr.toml`, the `driftr` key in `package.json`,
the standard `packageManager` field (npm, pnpm, yarn and bun), `.nvmrc`, `.node-version`,
and `engines.node`. The first source that names a tool wins, except that
`engines.node` counts only when no other source pins Node.js anywhere up the
tree. Each tool is looked up separately, so a repo can pin Node.js in `.nvmrc` and pnpm in
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
driftr uninstall npm@10.9.2
driftr uninstall pnpm@9.15.0
driftr uninstall yarn@1.22.22
driftr uninstall bun@1.2.0
```

Removes the version directory from `~/.driftr/tools/<tool>/<version>/`.

**Notes:**

- If the version is the current global default, a warning is printed
- If a pin selects the version (in `.driftr.toml`, `package.json`, `.nvmrc` or `.node-version`, exact or partial) for the current directory or any recorded project, a warning naming that file is printed
- The version must be exact: `driftr uninstall node@22` names the installed version it would select instead of removing anything
- Cached archives in `~/.driftr/cache/` are not removed (they will be reused if you reinstall)

## driftr prune

Remove installed versions that nothing references.

```bash
driftr prune --dry-run     # show the plan, delete nothing
driftr prune               # show the plan, then ask before deleting
driftr prune --tool node -y
```

A version counts as referenced when it is the global default for its tool, when `DRIFTR_<TOOL>` in the current shell selects it, when the project in the current directory pins it, or when a recorded project pins it. Pins are resolved the same way the shims resolve them, so a partial pin such as `24`, an `lts/jod` alias, or an `engines.node` range protects the installed version it would actually select. Everything else is listed with its size and removed after you confirm.

**Scope:** `driftr pin` (including `--bump`, but not `--bump --dry-run`) and `driftr install` record the project they run in, in `~/.driftr/config/projects`, one directory per line. The directory recorded is the one holding the nearest pin, which is where the shims would read it from. Prune re-reads the pins of every recorded project that still exists, along with the current directory and the global default, and prints how many projects it checked. Shims never write to this file, so running `node` stays free of writes.

Prune does not scan the disk. A project pinned before driftr recorded projects, or pinned only by editing `.nvmrc` or `package.json` by hand, is unknown until you run `driftr install` (or `driftr pin`) there once. Until then a version only it pins is offered for removal. Start with `--dry-run`, which lists the projects it checked.

A recorded project whose directory is gone is forgotten on the next prune (not on a dry run). A recorded project whose pins cannot be read stops prune before anything is removed: its pins are unknown, so no version can be shown to be unused. Fix the file, or delete its line from `~/.driftr/config/projects` if the project is no longer used.

**Flags:**

- `--dry-run` prints the plan and the recorded projects, and exits without touching anything
- `-y`, `--yes` skips the confirmation prompt
- `--tool <name>` restricts the run to one tool

**Notes:**

- If one directory cannot be removed, the rest still are, and the failures are reported at the end with an `rm -rf` command for manual cleanup
- Cached archives in `~/.driftr/cache/` are not touched, use `driftr cache clean` for those

## driftr default

Set the global default version for a tool.

```bash
driftr default node@22.14.0
driftr default npm@10.9.2
driftr default pnpm@9.15.0
driftr default yarn@1.22.22
driftr default bun@1.2.0
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

This writes the version, along with every other tool the old config pins, in the other format and removes the old config.

**Requirements:**
- The version must already be installed
- `package.json` format requires an existing `package.json` file (run `npm init` first)
- `package.json` format supports `node`, `npm`, `pnpm`, `yarn`, and `bun`

**Behavior:**
- Anyone who clones the project and has Driftr set up will automatically use the pinned version
- The pinned version takes priority over the global default
- Nested directories inherit the pin until another config overrides it
- In non-interactive environments (CI), defaults to `.driftr.toml`

**Bumping existing pins:**

```bash
driftr pin --bump                 # every pinned tool, newest release on the same major line
driftr pin --bump node pnpm       # only these tools
driftr pin --bump --major         # allow a newer major line
driftr pin --bump --dry-run       # print old -> new, write nothing
```

`--bump` reads the pins the project already has, the same ones `driftr outdated`
reports, and rewrites each one in the file that holds it: `.driftr.toml`, the
`driftr` key in `package.json`, `.nvmrc` or `.node-version`. It takes tool names,
not versions; `driftr pin --bump node@22` is an error, because that is what plain
`driftr pin node@22` is for.

```
$ driftr pin --bump
node: 22.1.0 -> 22.21.1 in .nvmrc
pnpm: 9.0.0 -> 9.15.9 in .driftr.toml
✓ Bumped 2 pin(s). Run `driftr install` to install the new versions.
```

A pin keeps the precision it was written with:

| Pin | `--bump` | `--bump --major` |
|-----|----------|------------------|
| `22.14.0` | newest `22.x.y` | newest release overall |
| `22.14` | newest `22.x` | newest `x.y` overall |
| `22` | stays `22`, it already floats | newest major, e.g. `24` |

Pre-releases are never a target, and a pin never moves backwards. When upstream
has no release on the pin's major line, the pin stays and the output says to use
`--major`.

**What is not rewritten:**
- `engines.node` is a range owned by the package author. Driftr prints the newest release and leaves the range for you to raise
- `packageManager` uses the corepack format and can carry an integrity hash. Driftr prints the newer version and suggests `corepack use pnpm@<version>`
- Aliases such as `lts`, `lts/*`, `lts/iron` and `latest` already float, so they are reported and skipped

**Behavior:**
- Nothing is installed; run `driftr install` afterwards
- `.nvmrc` and `.node-version` change only the version: a leading `v`, comments and the trailing newline stay
- `.driftr.toml` and `package.json` are written the same way `driftr pin` writes them, so comments in `.driftr.toml` do not survive; in `package.json` the other keys keep their order and content, and the `driftr` value is written compact
- With nothing to bump, the command says so and exits 0. It exits non-zero only when a release source could not be reached

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

The snippet sets `DRIFTR_NODE` (or `DRIFTR_NPM`, `DRIFTR_PNPM`, `DRIFTR_YARN`, `DRIFTR_BUN`), which the resolver reads
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
driftr list npm      # list npm versions installed by driftr
driftr list pnpm     # list pnpm versions
driftr list yarn     # list yarn versions
driftr list bun      # list bun versions
```

`npx` lists npm and `pnpx` lists pnpm. `--pre` and `--limit` only apply with `--remote`.

Output example, in a project that pins 24 with 22.14.0 as the global default:

```
Installed node versions:
     20.11.0
   * 22.14.0
  >  24.0.0

  > = active (current directory)
  * = global default
```

`>` marks the version the current directory resolves to, `*` the global default. A legend line
is printed only for a marker that appears.

**Remote versions:**

```bash
driftr list --remote [tool]          # available versions (default: latest 30)
driftr list --remote --limit 10      # limit output to 10 versions
driftr list --remote --limit 0       # show all versions (can be 500+ for node)
driftr list --remote --pre pnpm      # include pre-releases (npm packages only)
```

`--remote` reads nodejs.org for node, the npm registry for npm, pnpm and yarn, and bun's GitHub
releases for bun. Installed versions are marked with `●`, the active version with `>`, and the
global default with `*`. Node.js LTS releases show their codename (e.g. `LTS: Jod`).

**Alias:** `driftr ls`

## driftr outdated

Compare the versions the current project uses against the newest releases on
nodejs.org and the npm registry.

```bash
driftr outdated                # every managed tool
driftr outdated --tool node    # just one
driftr outdated --pre          # include npm pre-releases
driftr outdated --exit-code    # exit 1 when something is behind (CI)
```

Output example:

```
TOOL  CURRENT  SOURCE                          LINE LATEST  LATEST   LTS      STATUS
node  22.14.0  project config                  22.21.0      24.11.0  22.21.0  behind: 22.21.0 in 22.x, latest 24.11.0
pnpm  9.0.0    project config (not installed)  9.15.9       10.4.0   -        newest in 9.x, newer major: 10.4.0
```

`LINE LATEST` is the newest release on the major version you are already on,
which is usually the upgrade you can take today. `LATEST` is the newest release
of any major. For node the `LTS` column shows the newest long-term support
release.

**Where `CURRENT` comes from:** the same resolution `driftr which` uses, so a
project pin wins over the global default. A pin that is not installed yet still
shows up, marked `(not installed)`. Tools with no pin and nothing installed are
left out.

**Behavior:**
- A pin like `node = "22"` or `lts/*` floats, so it is only reported as behind when a newer major line exists
- If one tool's release source cannot be reached, its row says `unknown` and the rest of the report still runs
- The command exits 0 unless every source failed, or `--exit-code` was given and something is behind

## driftr check

Check that every file pinning a tool version for the current directory agrees
with the one Driftr uses. Nothing is fetched from the network.

```bash
driftr check                # report, always exit 0
driftr check --exit-code    # exit 1 when a problem is found (CI)
```

Output example:

```
node  22 (resolves to 22.14.0)
  > .driftr.toml                 22
    .nvmrc                       20
    package.json (engines.node)  >=24
  ✗ .nvmrc says 20, but .driftr.toml says 22 and wins
      Change .nvmrc to agree with 22, or run `driftr pin node@20` if 20 is the version you want.
  ✗ node 22 from .driftr.toml resolves to 22.14.0, which is outside package.json (engines.node) ">=24"
      Run `driftr pin node@24`, or widen the range in package.json (engines.node).

pnpm  9.15.0
  > package.json (packageManager)  9.15.0

⚠ Found 2 problem(s).
```

**Which sources are listed:** every source in the [resolution order](#resolution-order) that
lives in a file, in the current directory and each parent, in the order the resolver reads
them. The first one, marked `>`, is the one in use. Sources in a parent directory are listed
too. `engines.node` ranges come last, since they count only when no other source pins Node.js
anywhere up the tree.

**When two pins agree:** when at least one version satisfies both. `22` and `22.14.0`
agree, `22.14` and `22.15.0` do not. `lts` agrees with any line known to be LTS,
`lts/<codename>` with its own line, and `latest` with everything, since what it means
depends on a release index the check does not fetch. Each source is compared with the
one in use, not with every other source.

**`engines.node`:** the version in use must satisfy every `engines.node` range on the way
up. For a partial pin or an alias that is the installed version it resolves to; with
nothing installed, the pin passes when any version it accepts is inside the range.

**Behavior:**
- A file Driftr cannot read or parse, a pin that is not a version, and an LTS codename this
  build does not know are reported as problems
- `DRIFTR_<TOOL>` and the global default are not project pins and are not listed
- The command exits 0 unless `--exit-code` was given and a problem was found

## driftr which

Show which binary Driftr would execute, and why.

```bash
driftr which node
driftr which npm
driftr which pnpm
driftr which yarn
driftr which bun
```

Output example:

```
Tool:    node
Version: 22.14.0
Binary:  /home/user/.driftr/tools/node/22.14.0/bin/node
Source:  project config
Project: /home/user/my-project
```

When nothing pins npm, `driftr which npm` names the Node.js version whose bundled npm runs.

`driftr which` also explains a bin from a global npm install, such as `driftr which tsc`. It
names the Node.js version the bin runs under; see [Global npm packages](#global-npm-packages).

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
- Driftr's flags end at the command name, so `--` is optional: in `driftr run node -v`, `-v` goes to node
- `--node` takes a full version, a partial one (`20`), `latest`, `lts` or `lts/<codename>`, and
  picks the newest installed match without a network call
- The `--` separator is required between flags and the command
- Exit codes are preserved
- `--node` always pins the Node.js runtime. For `yarn`/`pnpm`, and `npm` when it is
  pinned, which need Node.js to execute but have their own independently pinned
  version, only the Node.js they run under is affected — the tool's own version
  still resolves normally from project/global config (e.g. `driftr run --node
  20.11.0 -- yarn` runs your pinned/default yarn under Node 20.11.0). `bun` does not run on
  Node.js, so `--node` leaves its version alone
- Everything the command starts stays on that version: `npm`, `npx` and package bins are
  `#!/usr/bin/env node` scripts, and the scripts they run look `node` up on `PATH` too, so
  `driftr run` puts that version's `bin` directory first on `PATH` and sets `DRIFTR_NODE`, the
  same override `driftr use` sets, for the command. This holds even when another `node` comes
  before Driftr's shims on `PATH`
- A bin from a global npm install, such as `tsc`, comes from that version's global packages and
  never from another version's. A command that is not a Node.js package, such as `make`, runs
  as it is found on `PATH`
- `driftr run --node 20 -- npm install -g <package>` installs into Node 20 and refreshes the
  shims, like `npm install -g` run directly
- A signal sent to `driftr` reaches the command. The exception is SIGINT and SIGQUIT while
  `driftr run` is the terminal's foreground job: they are taken to be Ctrl-C and Ctrl-\, which
  the terminal delivers to the command itself, so a `kill -INT` aimed at `driftr` then is not
  passed on. Send SIGTERM, or signal the command's own process

## driftr setup

Initialize Driftr directories, generate shim scripts, and put the shims on your `PATH`.

```bash
driftr setup
```

**What it creates:**
- `~/.driftr/bin/` with shims for `node`, `npm`, `npx`, `pnpm`, `pnpx`, `yarn`, `bun`, plus one
  for every bin a global npm install left in an installed Node.js version
- `~/.driftr/tools/` for installed tool versions
- `~/.driftr/config/` for global settings
- `~/.driftr/cache/` for downloads

**PATH:** setup appends `export PATH="$HOME/.driftr/bin:$PATH"` (fish: `set -gx PATH ...`) to the
file every invocation of your shell reads, chosen from `$SHELL`: `~/.zshenv` for zsh
(`$ZDOTDIR/.zshenv` when set), `~/.bash_profile` for bash, `~/.config/fish/conf.d/driftr.fish`
for fish, and `~/.profile` otherwise. If the line is already there it writes nothing. If another
install shadows a shim in the current shell, it also appends a precedence guard to the interactive
rc file (`.zshrc`, `.bashrc` or fish `config.fish`). When it cannot write the file, it prints the
line to add by hand. Restart the shell afterwards.

Run this once after installing Driftr. After an upgrade the shims regenerate on their own (see
[Global npm packages](#global-npm-packages)), so running it again is optional.

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
driftr doctor --fix   # auto-fix PATH and shim problems
```

Runs 11 checks, in this order:

1. `~/.driftr/bin` is on `PATH`
2. no other install (Homebrew, Volta, …) comes earlier on `PATH` and shadows a shim
3. the PATH export is in the file every shell invocation reads, not only an interactive rc file
4. every shim exists and is executable
5. the shims point at the current driftr binary
6. the shims were written by this driftr version
7. a global default node version is set
8. every global default version is installed
9. no conflicting version manager is present: nvm (through `$NVM_DIR/nvm.sh`), or `fnm`, `volta`
   or `n` on `PATH`
10. how many versions of each tool are installed
11. npm, pnpm or yarn is not installed without any Node.js to run it

The `--fix` flag repairs the PATH and shim problems, checks 2 to 6. It adds a PATH export to the
correct target file (e.g. `.zshenv` for zsh users), appends a precedence guard to the interactive
rc file when a shim is shadowed, and regenerates the shims when one is missing, is not executable,
points at an old driftr binary, or was written by another version. A stale export in an old rc
file is reported as a warning and has to be removed by hand. Everything else needs manual action,
following the printed suggestion.

## driftr node

Configure and maintain pnpm's shared content-addressable store, so dependencies are stored once
and reused across every project on the machine. Driftr does not replace pnpm, npm or yarn; these
commands only set pnpm's own configuration and run pnpm.

```bash
driftr node doctor             # package manager, Node.js, corepack, pnpm and store settings
driftr node optimize           # point pnpm at the shared store
driftr node optimize --install # same, then run pnpm install
driftr node report             # project node_modules size vs shared store size
driftr node clean              # dry run: show what would be removed
driftr node clean --yes        # remove node_modules, reinstall, prune the store
```

**`driftr node doctor`** reports, for the current directory: the package manager (from the
`packageManager` field in `package.json`, else from the lockfile), the Node.js version, whether
corepack and pnpm are available, the `node_modules` size, and pnpm's store path and global
virtual store setting. It reports missing tools rather than failing on them.

**`driftr node optimize`** runs `corepack enable` when corepack is available, then sets pnpm's
global config: `store-dir` to `~/.driftr/stores/pnpm` and `enable-global-virtual-store` to
`true`. A setting that already has that value is left alone, so running it twice changes
nothing. It needs pnpm; if pnpm is missing after corepack, it stops and says to install it.
`--install` runs `pnpm install` afterwards.

**`driftr node report`** prints the size of the project's `node_modules` and of the pnpm store.
The store is shared machine-wide, so its size is not a per-project cost.

**`driftr node clean`** is a dry run by default and changes nothing. With `--yes` it removes
`node_modules`, runs `pnpm install` to restore it, and runs `pnpm store prune`. Without pnpm it
still removes `node_modules`, then warns and skips the reinstall and prune.

## driftr completion

Print a shell completion script.

```bash
# zsh
echo 'eval "$(driftr completion zsh)"' >> ~/.zshrc

# bash
echo 'eval "$(driftr completion bash)"' >> ~/.bashrc

# fish
driftr completion fish | source
```

`driftr completion powershell` exists as well, from cobra, though Driftr itself does not run on
Windows. `driftr completion <shell> --help` shows how to load each script permanently.

## Auto-install on shim call

When a shim resolves a version that is not installed, for example after cloning a project that
pins `node@22.14.0`, Driftr can install it on the spot:

```
$ node -v
node 22.14.0 (pinned in /home/user/my-project) is not installed
Install now? [Y/n]
```

The prompt appears only when stdin is a terminal. Enter, `y` or `yes` installs the version and
runs the command; anything else exits with status 1. Without a terminal (scripts, CI) the shim
fails with the not-installed error and the `driftr install` command to run.

To install without asking, set `auto_install = true` at the top level of
`~/.driftr/config/config.toml`. No command writes this setting; edit the file by hand:

```toml
auto_install = true
```

With it set, the shim installs a missing version without asking, with or without a terminal.

## driftr self-update

Update Driftr to the latest version.

```bash
driftr self-update
```

It only updates when the latest release is newer than the running version, so a build ahead of
the release is never downgraded. A development build (`dev`) is refused; install a release instead.

After a successful update, automatically migrates PATH configuration if it was placed in an
interactive-only file (e.g. `.zshrc`) to the universal target (`.zshenv` for zsh). Prints a
note about any stale entries in old rc files — safe to remove manually.

## Pinning npm

Every Node.js release bundles an npm, and without a pin that is what the `npm` and `npx` shims
run, as they always have. To hold a project on one npm, for example to keep its lockfile format
stable across Node.js upgrades, pin it like any other tool:

```bash
driftr install npm@10
driftr pin npm@10.9.2
npm -v    # 10.9.2, whatever node is in use
```

A `packageManager` field such as `"npm@10.9.2"` pins it too, below the `driftr` key. Driftr
installs npm from the npm registry tarball, checks its SHA-512 integrity, and keeps it in
`~/.driftr/tools/npm/<version>/`. The shims run its `npm-cli.js` or `npx-cli.js` under the
Node.js version the directory resolves to, so `--node` and `DRIFTR_NODE` still choose the
runtime.

`npm install -g` under a pinned npm still installs into that Node.js version, the same place the
bundled npm uses, so the shims for global package bins work as described below.

A pinned npm covers the `npm` and `npx` you run, through the shims. Two kinds of nested call
find the Node.js `bin` directory first on `PATH`, and so the bundled npm: an `npm` that a global
package bin starts, and one that a script under `driftr run --node` starts.

## Global npm packages

`npm install -g` works as usual. The package lands in the Node.js version npm ran under, and once
npm exits Driftr writes a shim for each bin the package added to `~/.driftr/bin/`. `tsc`, `nest`
or `corepack` then run from any directory:

```bash
npm install -g typescript
tsc --version
driftr which tsc
```

A bin runs under the Node.js version it was installed with. When the version the current
directory resolves to has the bin, that one runs it. Otherwise the newest installed version that
has the bin runs it. A package with a native addon is built for one Node.js ABI, so running it
under a different version could fail to load.

`npm uninstall -g`, `driftr uninstall node@<version>` and `driftr prune` remove the shims of bins
that no installed version provides any more.

After Driftr is upgraded, by Homebrew, `driftr self-update` or the install script, the first
call to any shim or `driftr` command regenerates the shims once. Bins installed before the upgrade
get their shims then, without running `driftr setup`. If the shims cannot be rewritten, for
example because `~/.driftr/bin` is not writable, tools keep running, `driftr` commands print a
warning, and `driftr doctor` reports the outdated shims until `driftr doctor --fix` succeeds.

On Linux, driftr 1.2.1 and earlier wrote shims that exec the versioned Homebrew Cellar path,
which `brew upgrade` normally deletes. The formula marks those older kegs so Homebrew keeps them
and the old shims keep working; once driftr has rewritten the shims, it removes the mark and the
next `brew cleanup` frees the old keg. Shims written since then point at the stable `bin/driftr`
symlink.

The shim dir comes first on `PATH`, so a package shim steps aside when the shell would find the
same name somewhere else, such as a `nest` installed through Homebrew's own npm or `pnpm add -g`:
it runs that one, as if the shim were not there. This is decided on every call, against the
`PATH` of that call. `driftr which nest` says when that happens. Remove the other install if you
want Driftr's copy to run instead.

Only npm's global installs are covered. `pnpm add -g` and `yarn global add` keep their own global
directories, outside Driftr's tool versions.

## Resolution Order

When you run a tool (`node`, `npm`, `npx`, `pnpm`, `pnpx`, `yarn`, or `bun`), Driftr resolves the version in this order:

| Priority | Source | When |
|----------|--------|------|
| 1 | Explicit `--node` flag | `driftr run --node 24 -- ...` |
| 2 | `DRIFTR_<TOOL>` variable | Set in the shell, usually via `driftr use` |
| 3 | Project `.driftr.toml` | Found in current or parent directory |
| 4 | `package.json` driftr key | Found in current or parent directory |
| 5 | `package.json` `packageManager` field (npm/pnpm/yarn/bun only) | Found in current or parent directory |
| 6 | `.nvmrc` (node only) | Found in current or parent directory; `lts`, `lts/*` and `lts/<codename>` resolve against installed versions |
| 7 | `.node-version` (node only) | Found in current or parent directory; same LTS aliases as `.nvmrc` |
| 8 | `package.json` `engines.node` (node only) | Nearest one, used only when 3-7 pin node in no directory up the tree |
| 9 | Global default | Set via `driftr default` |

Rows 3 to 8 are not a ranking across the whole tree. Each directory is checked for every source before moving to its parent; the closest directory with a pin wins. A `.nvmrc` in the current
directory wins over a `.driftr.toml` in a parent directory.

If no version is configured at any level, Driftr prints an actionable error.

**Tool resolution:**

- `npm` and `npx` resolve via the **npm** version when one is pinned or set as the default, and run under the resolved **node**, like yarn. With no npm configured anywhere they run the npm bundled with the resolved **node** version
- `pnpm` and `pnpx` resolve via the **pnpm** version, and run `pnpm.cjs` and `pnpx.cjs` from it under the resolved **node**
- `yarn` resolves via the **yarn** version, and also co-resolves **node** because yarn is a JS script that needs `node` to execute
- `bun` resolves via the **bun** version. It is a native binary, so it runs without `node` installed

## Typical Workflow

```bash
# One-time setup (writes the PATH line, e.g. to ~/.zshenv)
driftr setup
exec $SHELL   # or open a new terminal

# Install your toolchain
driftr install node@22
driftr install pnpm@9
driftr install yarn@1
driftr install bun@1

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
