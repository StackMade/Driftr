# Using driftr

This machine manages Node.js, npm, pnpm, yarn and bun with driftr. The commands `node`, `npm`,
`npx`, `pnpm`, `pnpx`, `yarn` and `bun` in `~/.driftr/bin` are shims: on every call they pick the
version the current directory asks for. Run them normally.

## Rules

- Do not change versions with nvm, fnm, volta, n or `brew install node`. They shadow the shims.
- Install pnpm and yarn with `driftr install`, not with `npm install -g` or corepack.
- `npm install -g <package>` is fine for other packages. It lands in the Node.js version npm ran
  under, and driftr adds a shim for its bins.
- Do not hardcode paths under `~/.driftr/tools`. Ask with `driftr which <tool>`.
- Commit the pin file when you pin a project, so everyone gets the same version.

## How a version is chosen

Highest priority first:

1. `driftr run --node <version>`
2. The `DRIFTR_<TOOL>` environment variable, e.g. `DRIFTR_NODE`
3. Project files: `.driftr.toml`, the `driftr` key in `package.json`, `packageManager`, `.nvmrc`,
   `.node-version`, then `engines.node`
4. The global default, set with `driftr default`

Project files are checked one directory at a time, walking up. The closest directory with a pin
wins, so an `.nvmrc` here beats a `.driftr.toml` in a parent.

## Common tasks

| Task | Command |
|------|---------|
| Install everything this project pins | `driftr install` |
| Install one version | `driftr install node@22`, `driftr install pnpm@9`, `driftr install node@lts` |
| See which binary runs and why | `driftr which node`, with `-v` for the full trace |
| Run one command on another Node.js | `driftr run --node 20 -- npm test` |
| Pin this project (install the version first) | `driftr pin node@22.14.0` |
| Move pins to the newest release, then install | `driftr pin --bump`, then `driftr install` |
| Find pin files that disagree | `driftr check --exit-code` |
| List installed or available versions | `driftr list node`, `driftr list --remote node` |
| Compare with the newest releases | `driftr outdated` |
| Diagnose the installation | `driftr doctor --exit-code` |

## Running without a terminal

Agent shells usually have no terminal, and that changes a few things:

- A shim that needs a missing version fails instead of offering to install it. Read the error and
  run the `driftr install` command it names.
- `driftr use` only affects the shell that evals it, and each agent command may get a new shell.
  Use `driftr run --node <version> -- <command>` instead.
- `driftr pin` in a project with no pin yet writes `.driftr.toml` without asking.
- `driftr prune` needs `--yes` to remove anything. `driftr node clean` is a dry run without `--yes`.

## Ask the user first

These change state outside the project:

- `driftr default` changes the version every project without a pin uses.
- `driftr uninstall`, `driftr prune --yes` and `driftr node clean --yes` delete files. Show
  what will go first: `driftr prune --dry-run` for prune, plain `driftr node clean` for clean, and
  the exact version for uninstall.
- `driftr setup` and `driftr doctor --fix` edit shell startup files.
- `driftr self-update` replaces the driftr binary.

## When something is wrong

- `node` is not found, or `command -v node` is outside `~/.driftr/bin`: run `driftr doctor`. The
  agent's shell may lack the PATH entry, and `~/.driftr/bin/node` works directly in the meantime.
- Wrong version: run `driftr which node -v`, then `driftr check`.
- Driftr errors usually name the command that fixes them. Run `driftr <command> --help` for details.
