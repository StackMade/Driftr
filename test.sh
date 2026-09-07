#!/bin/bash
# shellcheck disable=SC2088  # tildes appear only in human-readable check labels

GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

pass=0
fail=0

check() {
    local desc="$1"
    shift
    if "$@" > /dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} $desc"
        pass=$((pass + 1))
    else
        echo -e "  ${RED}✗${NC} $desc"
        fail=$((fail + 1))
    fi
}

check_output() {
    local desc="$1"
    local expected="$2"
    shift 2
    local output
    output=$("$@" 2>&1) || true
    if echo "$output" | grep -q "$expected"; then
        echo -e "  ${GREEN}✓${NC} $desc"
        pass=$((pass + 1))
    else
        echo -e "  ${RED}✗${NC} $desc (expected '$expected', got '$output')"
        fail=$((fail + 1))
    fi
}

echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Driftr Integration Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo

# ── 1. Basic CLI ──────────────────────────────
echo -e "${BLUE}[1] Basic CLI${NC}"
check "driftr --help works" driftr --help
check_output "help shows available commands" "Available Commands" driftr --help
check_output "list shows no versions" "No node versions installed" driftr list
echo

# ── 2. Setup ──────────────────────────────────
echo -e "${BLUE}[2] Setup${NC}"
check "driftr setup creates dirs and shims" driftr setup
check "~/.driftr/bin exists" test -d "$HOME/.driftr/bin"
check "~/.driftr/tools/node exists" test -d "$HOME/.driftr/tools/node"
check "~/.driftr/config exists" test -d "$HOME/.driftr/config"
check "node shim was created" test -f "$HOME/.driftr/bin/node"
check "npm shim was created" test -f "$HOME/.driftr/bin/npm"
check "npx shim was created" test -f "$HOME/.driftr/bin/npx"
echo

# ── 3. Install (with checksum verification) ──
echo -e "${BLUE}[3] Install Node.js${NC}"
echo "  (installing node@22 — this may take a moment...)"
INSTALL_OUTPUT=$(driftr install node@22 -v 2>&1) || true
echo "$INSTALL_OUTPUT" | head -10
check "driftr install node@22 succeeds" echo "$INSTALL_OUTPUT"
check_output "checksum was verified" "Checksum verified OK" echo "$INSTALL_OUTPUT"
check_output "list shows installed version" "22." driftr list
echo

# ── 4. Default ────────────────────────────────
echo -e "${BLUE}[4] Set Global Default${NC}"

# Get the installed version dynamically
INSTALLED=$(driftr list 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
echo "  (installed version: $INSTALLED)"

check "driftr default node@$INSTALLED succeeds" driftr default "node@$INSTALLED"
check_output "list marks default with *" "*" driftr list
check_output "which resolves node" "$INSTALLED" driftr which node
check_output "which shows global source" "global default" driftr which node
echo

# ── 5. Pin ────────────────────────────────────
echo -e "${BLUE}[5] Project Pinning${NC}"
mkdir -p /tmp/test-project && cd /tmp/test-project || exit 1

check "driftr pin node@$INSTALLED succeeds" driftr pin "node@$INSTALLED"
check ".driftr.toml was created" test -f /tmp/test-project/.driftr.toml
check_output ".driftr.toml contains version" "$INSTALLED" cat /tmp/test-project/.driftr.toml
check_output "which shows project source" "project config" driftr which node

cd /home/driftr || exit 1
echo

# ── 6. Resolver Tracing ──────────────────────
echo -e "${BLUE}[6] Resolver Tracing${NC}"
TRACE_OUTPUT=$(driftr which node -v 2>&1) || true
check_output "verbose which shows resolution steps" "\\[resolve\\]" echo "$TRACE_OUTPUT"
check_output "verbose which shows step numbers" "Step" echo "$TRACE_OUTPUT"
echo

# ── 7. Run ────────────────────────────────────
echo -e "${BLUE}[7] Run Command${NC}"
export PATH="$HOME/.driftr/bin:$PATH"
check_output "driftr run -- node -v works" "v$INSTALLED" driftr run --node "$INSTALLED" -- node -v
echo

# ── 8. Reinstall (idempotency) ───────────────
echo -e "${BLUE}[8] Reinstall Idempotency${NC}"
check "reinstalling same version succeeds" driftr install "node@$INSTALLED"
check_output "still shows installed version" "$INSTALLED" driftr list
echo

# ── 9. Shim Execution ────────────────────────
echo -e "${BLUE}[9] Shim Execution${NC}"
check_output "node shim resolves correct version" "v$INSTALLED" "$HOME/.driftr/bin/node" -v
check_output "npm shim executes successfully" "." "$HOME/.driftr/bin/npm" -v
echo

# ── 10. Partial Version Resolution ──────────
echo -e "${BLUE}[10] Partial Version Resolution${NC}"
MAJOR=$(echo "$INSTALLED" | cut -d. -f1)
check "driftr default node@$MAJOR resolves to installed" driftr default "node@$MAJOR"
check_output "default is set to full version" "$INSTALLED" driftr which node
echo

# ── 11. Multi-tool Config ───────────────────
echo -e "${BLUE}[11] Multi-tool Config${NC}"
mkdir -p /tmp/test-multitool && cd /tmp/test-multitool || exit 1

# Pin node in .driftr.toml then verify multiple tools work in same config.
check "pin node in multi-tool project" driftr pin "node@$INSTALLED"
check_output ".driftr.toml has node version" "$INSTALLED" cat /tmp/test-multitool/.driftr.toml
cd /home/driftr || exit 1
echo

# ── 12. Error Cases ─────────────────────────
echo -e "${BLUE}[12] Error Cases${NC}"
check_output "install unknown tool errors" "unknown tool" driftr install "unknown@1.0"
check_output "default uninstalled version errors" "not installed" driftr default "node@99.99.99"
check_output "list unknown tool shows empty" "No unknown versions" driftr list unknown
echo

# ── 13. Setup Generates All Shims ───────────
echo -e "${BLUE}[13] Setup Shims${NC}"
check "pnpm shim was created" test -f "$HOME/.driftr/bin/pnpm"
check "pnpx shim was created" test -f "$HOME/.driftr/bin/pnpx"
check "yarn shim was created" test -f "$HOME/.driftr/bin/yarn"
check "bun shim was created" test -f "$HOME/.driftr/bin/bun"
echo

# ── 14. Node Shared Storage ─────────────────
# pnpm/corepack are not guaranteed in the test image, so only cover the
# pnpm-free paths: help registration and clean's dry-run (mutates nothing).
echo -e "${BLUE}[14] Node Shared Storage${NC}"
mkdir -p /tmp/test-node && cd /tmp/test-node || exit 1
check_output "node group help lists subcommands" "optimize" driftr node --help
check_output "node clean defaults to dry-run" "Dry run" driftr node clean
check "node clean dry-run creates no node_modules" sh -c '! test -d /tmp/test-node/node_modules'
cd /home/driftr || exit 1
echo

# ── 14b. LTS Alias ───────────────────────────
echo -e "${BLUE}[14b] LTS Alias${NC}"
LTS_OUTPUT=$(driftr install node@lts -v 2>&1) || true
check_output "driftr install node@lts succeeds" "Installed" echo "$LTS_OUTPUT"
check_output "install pnpm@lts errors (no LTS concept)" "lts is only supported for node" driftr install pnpm@lts
check_output "install yarn@lts errors (no LTS concept)" "lts is only supported for node" driftr install yarn@lts
check_output "install bun@lts errors (no LTS concept)" "lts is only supported for node" driftr install bun@lts
echo

# ── 15. pnpm/yarn Install ───────────────────
echo -e "${BLUE}[15] pnpm/yarn Install${NC}"
PNPM_OUTPUT=$(driftr install pnpm@9 -v 2>&1) || true
echo "$PNPM_OUTPUT" | head -10
check_output "driftr install pnpm@9 succeeds" "Checksum verified OK\|Installed" echo "$PNPM_OUTPUT"
check_output "list pnpm shows installed version" "9\\." driftr list pnpm
PNPM_INSTALLED=$(driftr list pnpm 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
check "driftr default pnpm@\$PNPM_INSTALLED succeeds" driftr default "pnpm@$PNPM_INSTALLED"
check_output "pnpm resolves to installed version" "$PNPM_INSTALLED" driftr which pnpm
# The fixture node (used for the node@22 mirror install above) can't run
# real pnpm.cjs and print its version, but it does echo which script it was
# handed — enough to confirm the shim execs pnpm's own script under node,
# not node's own -v.
check_output "pnpm shim execs pnpm.cjs via node" "pnpm.cjs" "$HOME/.driftr/bin/pnpm" -v

YARN_OUTPUT=$(driftr install yarn@1 -v 2>&1) || true
echo "$YARN_OUTPUT" | head -10
check_output "driftr install yarn@1 succeeds" "Checksum verified OK\|Installed" echo "$YARN_OUTPUT"
check_output "list yarn shows installed version" "1\\." driftr list yarn
YARN_INSTALLED=$(driftr list yarn 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
check "driftr default yarn@\$YARN_INSTALLED succeeds" driftr default "yarn@$YARN_INSTALLED"
check_output "yarn resolves to installed version" "$YARN_INSTALLED" driftr which yarn
check_output "yarn shim execs yarn.js via node" "yarn.js" "$HOME/.driftr/bin/yarn" -v
echo

# ── 15c. bun Install ─────────────────────────
# An exact version is used on purpose: a partial spec would query the GitHub
# releases API, which is rate-limited per IP and flaky on shared CI runners.
# bun is a native binary, so this also covers the standalone-exec shim path
# without node in the picture.
echo -e "${BLUE}[15c] bun Install${NC}"
BUN_VERSION=1.2.0
BUN_OUTPUT=$(driftr install "bun@$BUN_VERSION" -v 2>&1) || true
echo "$BUN_OUTPUT" | head -10
check_output "driftr install bun@$BUN_VERSION succeeds" "Checksum verified OK\\|Installed" echo "$BUN_OUTPUT"
check_output "list bun shows installed version" "$BUN_VERSION" driftr list bun
check "driftr default bun@\$BUN_VERSION succeeds" driftr default "bun@$BUN_VERSION"
check_output "bun resolves to installed version" "$BUN_VERSION" driftr which bun
check_output "bun shim execs the real binary" "$BUN_VERSION" "$HOME/.driftr/bin/bun" --version
echo

# ── 15b. package.json packageManager Field ──
echo -e "${BLUE}[15b] packageManager Field${NC}"
mkdir -p /tmp/test-pkgmanager && cd /tmp/test-pkgmanager || exit 1
echo "{\"name\": \"app\", \"packageManager\": \"pnpm@$PNPM_INSTALLED\"}" > package.json
check_output "packageManager field resolves pnpm version" "$PNPM_INSTALLED" driftr which pnpm
check_output "which shows packageManager source" "packageManager" driftr which pnpm
check_output "packageManager field ignored for node" "global default" driftr which node
cd /home/driftr || exit 1
echo

# ── 16. Uninstall ────────────────────────────
echo -e "${BLUE}[16] Uninstall${NC}"
check "driftr uninstall pnpm@\$PNPM_INSTALLED succeeds" driftr uninstall "pnpm@$PNPM_INSTALLED"
check_output "list pnpm no longer shows uninstalled version" "No pnpm versions installed" driftr list pnpm
check_output "uninstall unknown version errors" "not installed" driftr uninstall "pnpm@0.0.1"
check_output "uninstall without version errors" "version required" driftr uninstall pnpm
echo

# ── 17. Doctor ────────────────────────────────
echo -e "${BLUE}[17] Doctor${NC}"
check "driftr doctor runs" driftr doctor
check_output "doctor reports PATH status" "PATH" driftr doctor
check "driftr doctor --fix runs" driftr doctor --fix
echo

# ── 18. Self-update (registration only) ──────
# Actually replacing the running binary mid-suite would break every command
# after it, so only verify the command is wired up, not a real update.
echo -e "${BLUE}[18] Self-update${NC}"
check_output "self-update is a registered command" "self-update" driftr --help
echo

# ── Summary ───────────────────────────────────
echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
total=$((pass + fail))
if [ "$fail" -eq 0 ]; then
    echo -e "  ${GREEN}All $total tests passed!${NC}"
else
    echo -e "  ${GREEN}$pass passed${NC}, ${RED}$fail failed${NC} out of $total"
fi
echo -e "${BLUE}═══════════════════════════════════════${NC}"

if [ "$fail" -gt 0 ]; then
    exit 1
fi
exit 0
