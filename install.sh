#!/bin/sh
# Driftr installer — downloads the latest release from GitHub and sets up PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh
#
# Environment variables (set them for sh, after the pipe:
#   curl -fsSL .../install.sh | DRIFTR_VERSION=1.4.0 sh):
#   DRIFTR_VERSION     — pin a specific version (e.g. "1.4.0"), default: latest
#   DRIFTR_INSTALL_DIR — override install directory, default: ~/.driftr/bin

set -eu

REPO="StackMade/driftr"
DEFAULT_INSTALL_DIR="$HOME/.driftr/bin"
INSTALL_DIR="${DRIFTR_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

# --- helpers ----------------------------------------------------------------

log() {
    printf '%s\n' "$@"
}

err() {
    log "error: $*" >&2
    exit 1
}

need() {
    if ! command -v "$1" >/dev/null 2>&1; then
        err "$1 is required but not found"
    fi
}

# Download a URL to a local file. Prefers curl, falls back to wget.
download() {
    url="$1"
    dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        err "curl or wget is required"
    fi
}

# Download a URL and print to stdout.
download_stdout() {
    url="$1"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$url"
    else
        err "curl or wget is required"
    fi
}

# --- detect platform --------------------------------------------------------

detect_os() {
    os="$(uname -s)"
    case "$os" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       err "unsupported OS: $os" ;;
    esac
}

detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             err "unsupported architecture: $arch" ;;
    esac
}

# --- resolve version --------------------------------------------------------

resolve_version() {
    if [ -n "${DRIFTR_VERSION:-}" ]; then
        # Strip leading "v" if present.
        echo "$DRIFTR_VERSION" | sed 's/^v//'
        return
    fi

    tag=$(download_stdout "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"tag_name": *"v?([^"]+)".*/\1/')

    if [ -z "$tag" ]; then
        err "could not determine latest version (GitHub API rate limit?)"
    fi

    echo "$tag"
}

# --- checksum verification --------------------------------------------------

verify_checksum() {
    archive="$1"
    checksums_file="$2"
    filename="$(basename "$archive")"

    # Exact second-field match — a substring grep could match the wrong line.
    expected=$(awk -v f="$filename" 'NF == 2 && $2 == f { print $1; exit }' "$checksums_file")
    if [ -z "$expected" ]; then
        err "no checksum found for $filename in checksums.txt"
    fi
    case "$expected" in
        *[!0-9a-f]*) err "malformed checksum for $filename in checksums.txt" ;;
    esac
    if [ "${#expected}" -ne 64 ]; then
        err "malformed checksum for $filename in checksums.txt"
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive" | awk '{print $1}')
    else
        err "sha256sum or shasum is required for checksum verification"
    fi

    if [ "$actual" != "$expected" ]; then
        err "checksum mismatch for $filename (expected $expected, got $actual)"
    fi
}

# --- signature verification -------------------------------------------------

# Check checksums.txt against the Sigstore bundle when cosign is installed.
# With cosign, a missing or invalid signature stops the install: someone who
# replaced a release could drop the bundle too.
verify_signature() {
    checksums_file="$1"
    bundle_url="$2"
    version="$3"

    if ! command -v cosign >/dev/null 2>&1; then
        log "cosign not found, so the release signature was not checked."
        return 0
    fi

    bundle="${checksums_file}.sigstore.json"
    download "$bundle_url" "$bundle" || err "cannot download the signature of v${version}, refusing to install"

    # Matches the release workflow for exactly this tag, so an older release's
    # signature cannot be replayed. GitHub ignores case in repository names.
    escaped=$(printf '%s' "$version" | sed 's/[.]/\\./g')
    identity='^https://github\.com/(?i:stackmade/driftr)/\.github/workflows/release\.yml@refs/tags/v'"$escaped"'$'

    log "Verifying signature..."
    cosign verify-blob --bundle "$bundle" \
        --certificate-identity-regexp "$identity" \
        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        "$checksums_file" >/dev/null || err "signature verification failed for v${version}, refusing to install"
}

# --- main -------------------------------------------------------------------

main() {
    need tar
    need uname

    os=$(detect_os)
    arch=$(detect_arch)
    version=$(resolve_version)

    archive_name="driftr_${version}_${os}_${arch}.tar.gz"
    base_url="https://github.com/${REPO}/releases/download/v${version}"

    log "Installing driftr v${version} (${os}/${arch})..."

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    log "Downloading ${archive_name}..."
    download "${base_url}/${archive_name}" "${tmpdir}/${archive_name}"
    download "${base_url}/checksums.txt" "${tmpdir}/checksums.txt"

    verify_signature "${tmpdir}/checksums.txt" "${base_url}/checksums.txt.sigstore.json" "$version"

    log "Verifying checksum..."
    verify_checksum "${tmpdir}/${archive_name}" "${tmpdir}/checksums.txt"

    log "Extracting..."
    tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir"

    mkdir -p "$INSTALL_DIR"
    # Copy next to the target, then rename over it: writing into a driftr that
    # a shim is running fails on Linux (ETXTBSY), and a shim started mid-copy
    # would run a half-written binary.
    cp "${tmpdir}/driftr" "${INSTALL_DIR}/.driftr.new"
    chmod +x "${INSTALL_DIR}/.driftr.new"
    mv -f "${INSTALL_DIR}/.driftr.new" "${INSTALL_DIR}/driftr"

    log "Installed driftr to ${INSTALL_DIR}/driftr"

    # Run setup to create directories and generate shims.
    "${INSTALL_DIR}/driftr" setup

    # Configure PATH in shell profile if not already present.
    configure_path

    log ""
    log "driftr v${version} installed successfully!"
    log ""
    case ":${PATH:-}:" in
        *":${INSTALL_DIR}:"*)
            log "Run 'driftr --version' to verify."
            ;;
        *)
            log "Open a new shell, or run this in the current shell:"
            log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            ;;
    esac
    log ""
    log "To enable shell completions:"
    log "  # zsh"
    log "  echo 'eval \"\$(driftr completion zsh)\"' >> ~/.zshrc"
    log "  # bash"
    log "  echo 'eval \"\$(driftr completion bash)\"' >> ~/.bashrc"
    log "  # fish"
    log "  driftr completion fish > ~/.config/fish/completions/driftr.fish"
}

configure_path() {
    shell_name="$(basename "${SHELL:-/bin/sh}")"

    # Fish uses different syntax and a different config location.
    if [ "$shell_name" = "fish" ]; then
        fish_conf_dir="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d"
        fish_profile="${fish_conf_dir}/driftr.fish"
        if [ -f "$fish_profile" ] && grep -qF "$INSTALL_DIR" "$fish_profile"; then
            return
        fi
        mkdir -p "$fish_conf_dir"
        # shellcheck disable=SC2016  # $PATH must stay literal; fish expands it at runtime
        # Single-quoted so a path with spaces stays one entry; fish only
        # treats \ and ' specially inside single quotes, so escape those.
        fish_dir=$(printf '%s' "$INSTALL_DIR" | sed "s/[\\\\']/\\\\&/g")
        printf "# Driftr\nset -gx PATH '%s' \$PATH\n" "$fish_dir" >> "$fish_profile"
        log "Added ${INSTALL_DIR} to PATH in ${fish_profile}"
        return
    fi

    path_line="export PATH=\"${INSTALL_DIR}:\$PATH\""

    # Pick the profile that is read by ALL invocations of the shell (not just
    # interactive ones) so driftr works in scripts, IDE terminals, and cron.
    #   - zsh:  .zshenv is sourced on every invocation
    #   - bash: a login shell reads the first of .bash_profile, .bash_login and
    #           .profile that exists; non-interactive children inherit PATH from
    #           the login shell environment. Creating .bash_profile next to an
    #           existing .profile would hide that .profile from every login.
    case "$shell_name" in
        zsh)
            # zsh reads .zshenv from ZDOTDIR as it was at startup; when
            # ~/.zshenv is what sets ZDOTDIR, $ZDOTDIR/.zshenv is never read.
            profile="$HOME/.zshenv"
            if [ -n "${ZDOTDIR:-}" ] && ! grep -qE '^[[:space:]]*(export[[:space:]]+)?ZDOTDIR=' "$HOME/.zshenv" 2>/dev/null; then
                profile="$ZDOTDIR/.zshenv"
            fi
            ;;
        bash)
            profile="$HOME/.bash_profile"
            for candidate in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
                if [ -f "$candidate" ]; then
                    profile="$candidate"
                    break
                fi
            done
            ;;
        *)
            profile="$HOME/.profile"
            ;;
    esac

    # Write to primary profile for non-interactive shell coverage.
    if [ -f "$profile" ] && grep -qF "$INSTALL_DIR" "$profile"; then
        log "${INSTALL_DIR} already configured in ${profile}"
    else
        for stale in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile" "$HOME/.bash_profile"; do
            [ "$stale" = "$profile" ] && continue
            # On macOS zsh we intentionally also write to .zshrc below — skip the warning for it.
            [ "$shell_name" = "zsh" ] && [ "$(uname)" = "Darwin" ] && [ "$stale" = "$HOME/.zshrc" ] && continue
            if [ -f "$stale" ] && grep -qF "$INSTALL_DIR" "$stale"; then
                log "note: ${INSTALL_DIR} is also in ${stale} — consider removing that entry to avoid duplicate PATH"
            fi
        done

        printf '\n# Driftr\n%s\n' "$path_line" >> "$profile"
        log "Added ${INSTALL_DIR} to PATH in ${profile}"
    fi

    # macOS path_helper workaround: /usr/libexec/path_helper (invoked via brew shellenv
    # in ~/.zprofile) rebuilds PATH for every login shell, pushing .zshenv entries
    # after /opt/homebrew/bin. This causes Homebrew's node to shadow driftr shims.
    # Appending to .zshrc (sourced last in interactive sessions) re-prepends the
    # install dir so shims always win.
    if [ "$shell_name" = "zsh" ] && [ "$(uname)" = "Darwin" ]; then
        zshrc="${ZDOTDIR:-$HOME}/.zshrc"
        if [ -f "$zshrc" ] && grep -qF "$INSTALL_DIR" "$zshrc"; then
            : # already present
        else
            printf '\n# Driftr — re-export after path_helper/brew shellenv so shims win\n%s\n' "$path_line" >> "$zshrc"
            log "Added ${INSTALL_DIR} to PATH in ${zshrc} (macOS path_helper override)"
        fi
    fi
}

main
