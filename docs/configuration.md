# Configuration

Driftr uses configuration at two levels: global (`config.toml`) and per-project (`.driftr.toml` or `package.json`).

## Global Configuration

**Location:** `~/.driftr/config/config.toml`

This file stores your global default tool versions. It is created and managed by `driftr default`.

### Format

```toml
auto_install = true  # install missing versions without prompting

[default]
node = "22.14.0"

[default.tools]
pnpm = "9.15.0"
yarn = "1.22.22"
bun = "1.2.3"
```

### Fields

| Section           | Key            | Type   | Description                                             |
|-------------------|----------------|--------|---------------------------------------------------------|
| (top-level)       | `auto_install` | bool   | Automatically install missing versions (default: false) |
| `[default]`       | `node`         | string | Global default Node.js version                          |
| `[default.tools]` | `pnpm`         | string | Global default pnpm version                             |
| `[default.tools]` | `yarn`         | string | Global default yarn version                             |
| `[default.tools]` | `bun`          | string | Global default bun version                              |

### Example

After running:

```bash
driftr default node@22.14.0
driftr default pnpm@9.15.0
```

The config file will contain both tool defaults.

## Project Configuration

Driftr supports two project config formats. On first `driftr pin`, you choose which to use. The choice is auto-detected on subsequent runs.

### Option 1: `.driftr.toml` (recommended)

**Location:** `.driftr.toml` in the project root

```toml
[tools]
node = "22.14.0"
pnpm = "9.15.0"
```

| Section   | Key    | Type   | Description                                |
|-----------|--------|--------|--------------------------------------------|
| `[tools]` | `node` | string | Pinned Node.js version for this project    |
| `[tools]` | `pnpm` | string | Pinned pnpm version for this project       |
| `[tools]` | `yarn` | string | Pinned yarn version for this project       |
| `[tools]` | `bun`  | string | Pinned bun version for this project        |

### Option 2: `package.json`

**Location:** `driftr` key in an existing `package.json`

```json
{
  "name": "my-project",
  "driftr": {
    "node": "22.14.0",
    "pnpm": "9.15.0",
    "yarn": "1.22.22",
    "bun": "1.2.3"
  }
}
```

| Key            | Type   | Description                              |
|----------------|--------|------------------------------------------|
| `driftr.node`  | string | Pinned Node.js version for this project  |
| `driftr.pnpm`  | string | Pinned pnpm version for this project     |
| `driftr.yarn`  | string | Pinned yarn version for this project     |
| `driftr.bun`   | string | Pinned bun version for this project      |

This format is useful when you want to keep all project tooling config in `package.json` without an extra dotfile.

**Note:** `package.json` must already exist — Driftr will not create it. Run `npm init` first if needed.

### Migrating Between Formats

```bash
# Switch from current format to the other
driftr pin node@22.14.0 --migrate
```

This writes the version in the new format and removes the old config (deletes `.driftr.toml` or removes the `driftr` key from `package.json`).

### Directory Walk Behavior

A `DRIFTR_<TOOL>` variable in the environment (see [Environment Variables](#environment-variables)) short-circuits the walk described here. Without one, Driftr walks up from the current directory to the filesystem root. In each directory, it checks config files in priority order:

1. `.driftr.toml`
2. `package.json` (driftr key)
3. `package.json` (`packageManager` field, e.g. `"packageManager": "pnpm@9.15.0"`, pnpm/yarn/bun only)
4. `.nvmrc` (node only)
5. `.node-version` (node only)
6. `package.json` (`engines.node`, node only)

```
/home/user/my-project/packages/core/   <- cwd, no config
/home/user/my-project/packages/        <- no config
/home/user/my-project/                 <- .driftr.toml found! uses this
```

If multiple config files exist in the same directory, the priority order above applies. `.nvmrc` and `.node-version` are only used for Node.js version resolution. The `packageManager` field is read for pnpm, yarn and bun only (not node, since npm/npx always follow node's own version and node has no equivalent standard field); it must name an exact installed version, same as the `driftr` key — it's read-only, Driftr never writes it.

**LTS aliases.** `.nvmrc` and `.node-version` may hold `lts`, `lts/*`, or a codename such as `lts/jod`. Driftr resolves those against the versions you already have installed, never over the network, since the shim runs on every `node` call. `lts` and `lts/*` take the newest installed even-numbered major, because Node's LTS lines have been the even majors since v4. `lts/<codename>` takes the newest installed release of that codename's major, so `lts/iron` means 20 and `lts/jod` means 22. When nothing installed fits, you get the usual not-installed error with the command to run, which is also what lets auto-install step in. An unknown codename prints a warning and resolution moves on to the next source.

This means:

- You only need one config at the project root
- All subdirectories inherit the pinned version
- A nested config overrides the parent

### engines.node

The last project-level source is the standard `engines` object:

```json
{
  "engines": {
    "node": ">=18"
  }
}
```

Unlike the other sources, this is a range rather than one version, so Driftr takes the newest installed version that satisfies it. It never downloads anything here and never queries nodejs.org. If nothing installed fits the range, Driftr fails with an error naming a version you can install: `driftr install node@18` for `>=18`, or `driftr install node@lts` when the range has no lower bound.

Supported syntax:

| Form | Example |
|------|---------|
| Comparators | `>=18`, `>18`, `<21`, `<=20`, `=20.11.1` |
| Caret | `^20.9.0` |
| Tilde | `~20.9.0` |
| Wildcards | `18.x`, `18.*`, `*` |
| Bare version | `18`, `18.1`, `18.1.0` |
| AND (whitespace) | `>=16 <21` |
| OR | `18 \|\| 20` |

Not supported: hyphen ranges (`18 - 20`) and pre-release tags (`>=18.0.0-rc.1`). Driftr rejects those with an error naming the range instead of guessing what you meant.

Driftr only reads `engines.node`. `driftr pin` still writes the `driftr` key.

### Version Control

Your project config (`.driftr.toml` or `package.json`) **should be committed** to version control. This ensures all team members use the same tool versions.

```bash
git add .driftr.toml   # or package.json
git commit -m "Pin Node.js version with Driftr"
```

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `DRIFTR_NODE`, `DRIFTR_PNPM`, `DRIFTR_YARN`, `DRIFTR_BUN` | unset | Version for that tool in the current shell. Read before any project config, so it overrides `.driftr.toml`, `package.json`, `.nvmrc` and `.node-version`. Takes a full version, a partial one (`24`), or `latest`/`lts`, and must name a version you already have installed. `driftr use` prints the line that sets it. |
| `DRIFTR_NODE_MIRROR` | `https://nodejs.org/dist` | Alternative Node.js distribution mirror (corporate mirrors, air-gapped setups, hermetic tests). Must serve the same layout: `index.json`, `v<version>/SHASUMS256.txt`, and version tarballs. |
| `DRIFTR_NPM_REGISTRY` | `https://registry.npmjs.org` | Alternative npm registry for pnpm/yarn installs. Tarball URLs in registry metadata must point back at the same host. |
| `DRIFTR_BUN_RELEASES` | `https://api.github.com/repos/oven-sh/bun/releases` | Alternative source for the bun release list. Must answer with the same JSON shape: an array of objects carrying `tag_name`, `draft` and `prerelease`. |
| `DRIFTR_BUN_MIRROR` | `https://github.com/oven-sh/bun/releases/download` | Alternative host for bun release assets. Must serve `bun-v<version>/bun-<os>-<arch>.zip` and `bun-v<version>/SHASUMS256.txt`. |
| `DRIFTR_UPDATE_API` | `https://api.github.com/repos/stackmade/driftr` | Alternative source for the Driftr release index used by `driftr self-update`. `/releases/latest` is appended, and the answer must carry a `tag_name`. |
| `DRIFTR_UPDATE_MIRROR` | `https://github.com/stackmade/driftr/releases/download` | Alternative host for Driftr's own release assets. `/v<version>` is appended, and the directory must hold `driftr_<version>_<os>_<arch>.tar.gz` and `checksums.txt`. |

```bash
DRIFTR_NODE_MIRROR=https://npmmirror.com/mirrors/node driftr install node@22

eval "$(driftr use node@24)"   # sets DRIFTR_NODE for this shell
eval "$(driftr use --unset node)"
```

## Storage Layout

Driftr stores all data under `~/.driftr/`:

```
~/.driftr/
  bin/                        shim scripts
    node                      shell script -> driftr shim node
    npm                       shell script -> driftr shim npm
    npx                       shell script -> driftr shim npx
    pnpm                      shell script -> driftr shim pnpm
    pnpx                      shell script -> driftr shim pnpx
    yarn                      shell script -> driftr shim yarn
    bun                       shell script -> driftr shim bun
  tools/
    node/
      22.14.0/
        bin/node, npm, npx
    pnpm/
      9.15.0/
        bin/pnpm, pnpx (symlink)
    yarn/
      1.22.22/
        bin/yarn.js
        lib/
        package.json
    bun/
      1.2.3/
        bin/bun
  config/
    config.toml               global configuration
  cache/
    node-v22.14.0-*.tar.gz    cached Node.js archives
    pnpm-9.15.0-*             cached pnpm binaries
    yarn-1.22.22.tgz          cached yarn tarballs
    bun-v1.2.3-bun-*.zip      cached bun release zips
```

### Cache

Downloaded archives are cached in `~/.driftr/cache/`. Subsequent installs of the same version skip the download. To force a re-download, delete the cached archive:

```bash
rm ~/.driftr/cache/node-v22.14.0-*.tar.gz
driftr install node@22.14.0
```

## Future Compatibility

The configuration format is designed for extension. Future versions may add:

- Mirror configuration for custom download sources
