#!/bin/sh
# PATH + shim e2e tests.
#
# Phase 1 — PATH config: verify configure_path() writes the correct rc file
#   and that a fresh shell subprocess with a scrubbed environment finds
#   driftr's bin dir on PATH automatically.
#
# Phase 2 — Shim + version: install node@22, node@24, node@latest; verify
#   node/npm/npx resolve correctly per global default and per-project pin
#   (.driftr.toml and package.json), and that an uninstalled pin exits non-zero.
#
# Prerequisites:
#   - driftr binary must be on PATH
#   - install.sh must exist in the current directory
#
# Environment:
#   TEST_SHELL  zsh | bash | fish | all (default: all)

set -eu

pass=0
fail_count=0

ok()   { printf '[PASS] %s\n' "$1"; pass=$((pass + 1)); }
fail() { printf '[FAIL] %s\n' "$1"; fail_count=$((fail_count + 1)); }

assert_cmd() {
    desc="$1"; shift
    if "$@" > /dev/null 2>&1; then ok "$desc"; else fail "$desc"; fi
}

assert_fail() {
    desc="$1"; shift
    if "$@" > /dev/null 2>&1; then fail "$desc (expected failure, got success)"; else ok "$desc"; fi
}

# ── setup ────────────────────────────────────────────────────────────────────

INSTALL_DIR="$HOME/.driftr/bin"

# Scrubbed PATH for zsh/bash env -i assertions: no /usr/local/bin (where the
# driftr system binary lives in Docker). Used only for PATH-config assertions.
CLEAN_PATH="/usr/bin:/usr/sbin:/bin:/sbin"

printf 'driftr PATH + shim e2e — shell: %s\n\n' "${TEST_SHELL:-all}"

driftr setup

# Unique sentinel in ~/.driftr/bin — proves bin dir is on PATH via rc file,
# not via /usr/local/bin where the driftr system binary is in Docker.
SENTINEL="driftr-path-e2e-ok"
printf '#!/bin/sh\necho ok\n' > "$INSTALL_DIR/$SENTINEL"
chmod +x "$INSTALL_DIR/$SENTINEL"

# Patch install.sh: disable the bare 'main' call at the last line so we can
# source function definitions and call configure_path() directly.
# Pre-defining main() { :; } before sourcing does NOT work — install.sh
# redefines main() after our definition, overriding the no-op.
PATCHED_INSTALL=/tmp/install-path-test.sh
sed 's/^main$/: # disabled by test/' install.sh > "$PATCHED_INSTALL"

configure_path_for() {
    shell_bin="$1"
    (
        export INSTALL_DIR
        SHELL="$shell_bin"
        export SHELL
        # shellcheck source=/dev/null
        . "$PATCHED_INSTALL"
        configure_path
    )
}

# ── shell execution helpers (phase 2) ────────────────────────────────────────

# PATH for shim-execution assertions: includes driftr binary dir (needed by
# shim scripts that exec 'driftr shim <tool>') but NOT the install dir.
# For fish we strip install dir from the full PATH instead of using env -i,
# because fish 4.x (Rust) needs HOME/TMPDIR/USER/XDG_DATA_DIRS to start.
DRIFTR_DIR="$(dirname "$(command -v driftr)")"
SHIM_CLEAN_PATH="${DRIFTR_DIR}:/usr/bin:/usr/sbin:/bin:/sbin"
PATH_NO_INSTALL="$(printf '%s' "$PATH" | tr ':' '\n' | grep -vF "$INSTALL_DIR" | tr '\n' ':' | sed 's/:$//')"

# shell_exec SHELL CMD — run CMD in a fresh shell with controlled PATH.
# The rc file (zshenv / bash_profile / fish conf.d) adds INSTALL_DIR on top.
shell_exec() {
    _sh="$1"; _cmd="$2"
    case "$_sh" in
        zsh)  env -i HOME="$HOME" PATH="$SHIM_CLEAN_PATH" zsh    -c "$_cmd" ;;
        bash) env -i HOME="$HOME" PATH="$SHIM_CLEAN_PATH" bash -l -c "$_cmd" ;;
        fish) env PATH="$PATH_NO_INSTALL" fish -c "$_cmd" ;;
        *)    return 1 ;;
    esac
}

assert_shell() {
    _desc="$1"; _sh="$2"; _cmd="$3"
    if shell_exec "$_sh" "$_cmd" > /dev/null 2>&1; then ok "$_desc"; else fail "$_desc"; fi
}

assert_shell_output() {
    _desc="$1"; _sh="$2"; _cmd="$3"; _pat="$4"
    _out=$(shell_exec "$_sh" "$_cmd" 2>&1) || true
    if printf '%s\n' "$_out" | grep -q "$_pat"; then
        ok "$_desc"
    else
        fail "$_desc (got: $_out)"
    fi
}

assert_shell_fail() {
    _desc="$1"; _sh="$2"; _cmd="$3"
    if shell_exec "$_sh" "$_cmd" > /dev/null 2>&1; then
        fail "$_desc (expected failure, got success)"
    else
        ok "$_desc"
    fi
}

# cd_then SHELL DIR CMD — build a shell-appropriate "cd DIR && CMD" string.
# fish does not support && for chaining (pre-3.0) — use '; and' instead.
# fish 3.0+ supports && but '; and' works across all fish versions.
cd_then() { _csh="$1"; _dir="$2"; _c="$3"
    if [ "$_csh" = "fish" ]; then printf "cd '%s'; and %s" "$_dir" "$_c"
    else                           printf "cd '%s' && %s"  "$_dir" "$_c"
    fi
}

# ── phase 1: PATH config ─────────────────────────────────────────────────────

run_path_tests_zsh() {
    ZSH_BIN=$(command -v zsh 2>/dev/null) || { printf '[SKIP] zsh not installed\n'; return; }
    printf '\n[PATH config — zsh]\n'
    # configure_path() uses ${ZDOTDIR:-$HOME}/.zshenv. Pin ZDOTDIR=HOME so the
    # assertion checks the same file that configure_path wrote to, even if the
    # runner environment has ZDOTDIR set to something else.
    ZDOTDIR="$HOME"; export ZDOTDIR
    ZSHENV_PATH="$ZDOTDIR/.zshenv"
    configure_path_for "$ZSH_BIN"
    assert_cmd "zsh: .zshenv written"              test -f "$ZSHENV_PATH"
    assert_cmd "zsh: .zshenv contains bin dir"     grep -qF "$INSTALL_DIR" "$ZSHENV_PATH"
    # No --no-rcs: verifies zsh auto-sources .zshenv on every invocation.
    assert_cmd "zsh: non-interactive sources .zshenv" \
        env -i HOME="$HOME" ZDOTDIR="$ZDOTDIR" PATH="$CLEAN_PATH" zsh -c "command -v $SENTINEL"
    assert_cmd "zsh: interactive sources .zshenv" \
        env -i HOME="$HOME" ZDOTDIR="$ZDOTDIR" PATH="$CLEAN_PATH" zsh -i -c "command -v $SENTINEL"
}

run_path_tests_bash() {
    BASH_BIN=$(command -v bash 2>/dev/null) || { printf '[SKIP] bash not installed\n'; return; }
    printf '\n[PATH config — bash]\n'
    # bash logins read the first of these that exists; useradd -m copies
    # /etc/skel/.profile on Debian, and creating .bash_profile would hide it.
    BASH_RC="$HOME/.bash_profile"
    for _f in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
        if [ -f "$_f" ]; then BASH_RC="$_f"; break; fi
    done
    configure_path_for "$BASH_BIN"
    assert_cmd "bash: $(basename "$BASH_RC") written"           test -f "$BASH_RC"
    assert_cmd "bash: $(basename "$BASH_RC") contains bin dir"  grep -qF "$INSTALL_DIR" "$BASH_RC"
    assert_cmd "bash: login shell resolves sentinel" \
        env -i HOME="$HOME" PATH="$CLEAN_PATH" bash -l -c "command -v $SENTINEL"
    # Non-interactive non-login bash does not read .bash_profile — documented
    # limitation. The assert_fail makes the contract executable.
    assert_fail "bash: non-interactive non-login does not resolve (expected)" \
        env -i HOME="$HOME" PATH="$CLEAN_PATH" bash -c "command -v $SENTINEL"
    assert_cmd "bash: child of login shell inherits PATH" \
        env -i HOME="$HOME" PATH="$CLEAN_PATH" bash -l -c "bash -c 'command -v $SENTINEL'"
}

run_path_tests_fish() {
    FISH_BIN=$(command -v fish 2>/dev/null) || { printf '[SKIP] fish not installed\n'; return; }
    printf '\n[PATH config — fish]\n'
    configure_path_for "$FISH_BIN"
    FISH_CONF="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/driftr.fish"
    assert_cmd "fish: conf.d/driftr.fish written"       test -f "$FISH_CONF"
    assert_cmd "fish: driftr.fish contains set -gx PATH" grep -q 'set -gx PATH' "$FISH_CONF"
    # fish 4.x needs HOME/TMPDIR/USER: use PATH stripping rather than env -i.
    PATH_WITHOUT_INSTALL="$(printf '%s' "$PATH" | tr ':' '\n' | grep -vF "$INSTALL_DIR" | tr '\n' ':' | sed 's/:$//')"
    assert_cmd "fish: non-interactive sources conf.d" \
        env PATH="$PATH_WITHOUT_INSTALL" fish -c "command -v $SENTINEL"
    assert_cmd "fish: type -q succeeds" \
        env PATH="$PATH_WITHOUT_INSTALL" fish -c "type -q $SENTINEL"
}

# ── phase 2: node version install ────────────────────────────────────────────

NODE22_FULL=""
NODE24_FULL=""

install_node_versions() {
    printf '\n[node install — 22, 24, latest]\n'
    driftr install node@22
    driftr install node@24
    driftr install node@latest

    # Capture exact installed full versions for deterministic pin tests.
    # 'driftr which node' outputs the binary path; extract semver from it.
    # Resolving the partial pin at test-time against the release index risks
    # mismatches if a newer patch is published between install and pin-check.
    driftr default node@22
    NODE22_FULL="$(driftr which node 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
    driftr default node@24
    NODE24_FULL="$(driftr which node 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"

    [ -n "$NODE22_FULL" ] || { printf '[FAIL] could not determine full version for node@22\n' >&2; exit 1; }
    [ -n "$NODE24_FULL" ] || { printf '[FAIL] could not determine full version for node@24\n' >&2; exit 1; }
    printf 'node versions ready (22=%s, 24=%s)\n' "$NODE22_FULL" "$NODE24_FULL"
}

# ── phase 2: shim + pin + auto-install tests per shell ───────────────────────

run_shim_tests() {
    _sh="$1"
    printf '\n[shim + pin — %s]\n' "$_sh"

    # global default node@22
    driftr default node@22
    assert_shell_output "$_sh: node@22 global — node -v"  "$_sh" 'node -v'        '^v22\.'
    assert_shell        "$_sh: node@22 global — npm -v"   "$_sh" 'npm -v'
    assert_shell        "$_sh: node@22 global — npx -v"   "$_sh" 'npx --version'

    # global default node@24
    driftr default node@24
    assert_shell_output "$_sh: node@24 global — node -v"  "$_sh" 'node -v'        '^v24\.'

    # global default node@latest
    driftr default node@latest
    assert_shell_output "$_sh: node@latest global — node -v" "$_sh" 'node -v'     '^v[0-9]'

    # pin via .driftr.toml overrides global (global=latest, pin=exact 22.x)
    # Use the full version captured at install time — partial version "22" in
    # the pin risks a resolver mismatch if a newer 22.x patch is published
    # between install and pin-check.
    _p1="$(mktemp -d)"
    printf '[tools]\nnode = "%s"\n' "$NODE22_FULL" > "$_p1/.driftr.toml"
    assert_shell_output "$_sh: .driftr.toml pin overrides global" \
        "$_sh" "$(cd_then "$_sh" "$_p1" 'node -v')" "^v${NODE22_FULL}$"
    rm -rf "$_p1"

    # pin via package.json driftr key (global=latest, pin=exact 24.x)
    _p2="$(mktemp -d)"
    printf '{"driftr":{"node":"%s"}}\n' "$NODE24_FULL" > "$_p2/package.json"
    assert_shell_output "$_sh: package.json pin overrides global" \
        "$_sh" "$(cd_then "$_sh" "$_p2" 'node -v')" "^v${NODE24_FULL}$"
    rm -rf "$_p2"

    # auto-install: pin version that is not installed — shim must exit non-zero.
    # node@18 is not installed (we only installed 22, 24, latest).
    _p3="$(mktemp -d)"
    printf '[tools]\nnode = "18"\n' > "$_p3/.driftr.toml"
    assert_shell_fail "$_sh: uninstalled pin exits non-zero (node@18)" \
        "$_sh" "$(cd_then "$_sh" "$_p3" 'node -v')"
    rm -rf "$_p3"
}

# ── dispatch ─────────────────────────────────────────────────────────────────

for_each_active_shell() {
    _fn="$1"
    case "${TEST_SHELL:-all}" in
        zsh)  if command -v zsh  > /dev/null 2>&1; then "$_fn" zsh;  else printf '[SKIP] zsh not installed\n';  fi ;;
        bash) if command -v bash > /dev/null 2>&1; then "$_fn" bash; else printf '[SKIP] bash not installed\n'; fi ;;
        fish) if command -v fish > /dev/null 2>&1; then "$_fn" fish; else printf '[SKIP] fish not installed\n'; fi ;;
        all)
            if command -v zsh  > /dev/null 2>&1; then "$_fn" zsh;  fi
            if command -v bash > /dev/null 2>&1; then "$_fn" bash; fi
            if command -v fish > /dev/null 2>&1; then "$_fn" fish; fi
            ;;
        *) printf 'error: unknown TEST_SHELL=%s\n' "$TEST_SHELL" >&2; exit 1 ;;
    esac
}

printf '\n════════════════ PHASE 1: PATH CONFIG ════════════════\n'
case "${TEST_SHELL:-all}" in
    zsh)  run_path_tests_zsh ;;
    bash) run_path_tests_bash ;;
    fish) run_path_tests_fish ;;
    all)
        run_path_tests_zsh
        run_path_tests_bash
        run_path_tests_fish
        ;;
    *) printf 'error: unknown TEST_SHELL=%s\n' "$TEST_SHELL" >&2; exit 1 ;;
esac

printf '\n════════════════ PHASE 2: SHIM + VERSION ════════════════\n'
if [ "${SKIP_SHIM_TESTS:-0}" = "1" ]; then
    # Node.js official binaries are glibc-only and do not run on musl libc.
    # Phase 1 already proves driftr itself works on musl. Phase 2 is skipped
    # on Alpine so the musl-compat job does not fail on a Node.js limitation.
    printf '[SKIP] Phase 2 skipped — Node.js glibc binaries incompatible with musl\n'
else
    install_node_versions
    for_each_active_shell run_shim_tests
fi

# ── summary ──────────────────────────────────────────────────────────────────

printf '\n========================================\n'
printf 'PATH + shim e2e: %d passed, %d failed\n' "$pass" "$fail_count"
printf '========================================\n'
[ "$fail_count" -eq 0 ]
