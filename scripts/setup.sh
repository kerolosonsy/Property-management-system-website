#!/usr/bin/env bash
set -euo pipefail

NO_START=0
SKIP_DEPS=0
DB_REQUEST=""
DATABASE_MODE=""
CURRENT_PHASE="argument parsing"
PACKAGE_MANAGER=""
OS_NAME=""
LAN_IP=""
HOST_NAME=""
APT_UPDATED=0
ADMIN_PASSWORD_GENERATED=0
LISTEN_ALL_INTERFACES=1
DOCKER_CMD=(docker)
DATABASE_PASSWORD_ACTION=""
DATABASE_TARGET_PASSWORD=""
DATABASE_TARGET_URL=""
OWNER_PASSWORD_ACTION=""
OWNER_TARGET_PASSWORD=""
OWNER_TARGET_URL=""
OWNER_STARTUP_PASSWORD=""
APP_PASSWORD_ACTION=""
APP_TARGET_PASSWORD=""
APP_TARGET_URL=""

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

unexpected_error() {
    printf 'ERROR: Setup failed during %s. Fix the error above, then rerun this command.\n' "$CURRENT_PHASE" >&2
    exit 1
}
trap unexpected_error ERR

usage() {
    printf 'Usage: bash scripts/setup.sh [--db=native|docker] [--no-start] [--skip-deps]\n' >&2
}

for arg in "$@"; do
    case "$arg" in
        --no-start) NO_START=1 ;;
        --skip-deps) SKIP_DEPS=1 ;;
        --db=native) DB_REQUEST="native" ;;
        --db=docker) DB_REQUEST="docker" ;;
        --db=*)
            usage
            printf 'ERROR: Invalid database mode: %s (expected native or docker)\n' "${arg#--db=}" >&2
            exit 2
            ;;
        *)
            usage
            printf 'ERROR: Unknown option: %s\n' "$arg" >&2
            exit 2
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
ENV_FILE="$ROOT/.env"
COMPOSE_FILE="$ROOT/infra/docker-compose.yml"
CERT_DIR="$ROOT/infra/certs"
CERT_FILE="$CERT_DIR/localhost.crt"
KEY_FILE="$CERT_DIR/localhost.key"
WEB_DIST="$ROOT/web/dist/web/browser"

heading() {
    CURRENT_PHASE="$1"
    printf '\n==> %s\n' "$1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

apt_candidate() {
    apt-cache policy "$1" 2>/dev/null | sed -n 's/^[[:space:]]*Candidate:[[:space:]]*//p' | sed -n '1p'
}

detect_package_manager() {
    local candidate_manager
    if [[ "$OS_NAME" == "macOS" ]]; then
        command_exists brew && PACKAGE_MANAGER=brew
        return 0
    fi
    for candidate_manager in apt-get dnf pacman zypper apk; do
        if command_exists "$candidate_manager"; then
            PACKAGE_MANAGER="$candidate_manager"
            return
        fi
    done
}

detect_lan_ip() {
    local interface_name=""
    if [[ "$OS_NAME" == "macOS" ]]; then
        if command_exists route; then
            interface_name="$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}')"
        fi
        if [[ -n "$interface_name" ]] && command_exists ipconfig; then
            LAN_IP="$(ipconfig getifaddr "$interface_name" 2>/dev/null || true)"
        fi
    else
        if command_exists ip; then
            LAN_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "src") {print $(i+1); exit}}')"
        fi
        if [[ -z "$LAN_IP" ]] && command_exists hostname; then
            LAN_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
        fi
    fi
    if [[ ! "$LAN_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
        fail "Could not determine the primary LAN IPv4 address. Connect this machine to the LAN, then rerun this command."
    fi
}

run_privileged() {
    if [[ "$(id -u)" -eq 0 ]]; then
        if ! "$@"; then
            fail "The privileged command failed. Run '$*' manually, resolve its error, then rerun this command."
        fi
        return
    fi
    if ! command_exists sudo || ! sudo -n true >/dev/null 2>&1; then
        fail "Installing system packages requires non-interactive sudo. Run 'sudo -v', configure sudo for this account, or install the listed dependencies manually and rerun with --skip-deps."
    fi
    if ! sudo -n "$@"; then
        fail "The privileged command failed. Run 'sudo $*' manually, resolve its error, then rerun this command."
    fi
}

install_packages() {
    case "$PACKAGE_MANAGER" in
        brew)
            local package
            for package in "$@"; do
                if brew list --formula "$package" >/dev/null 2>&1; then
                    brew upgrade "$package" || fail "Homebrew could not update $package."
                else
                    brew install "$package" || fail "Homebrew could not install $package."
                fi
            done
            return
            ;;
        apt-get)
            if [[ "$APT_UPDATED" -eq 0 ]]; then
                run_privileged apt-get update
                APT_UPDATED=1
            fi
            run_privileged env DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
            ;;
        dnf)
            run_privileged dnf --refresh install -y "$@"
            ;;
        pacman)
            run_privileged pacman -S --needed --noconfirm "$@"
            ;;
        zypper)
            run_privileged zypper --non-interactive refresh
            run_privileged zypper --non-interactive install "$@"
            ;;
        apk)
            run_privileged apk --update-cache add --upgrade --no-interactive "$@"
            ;;
        *)
            fail "No supported package manager was found. Install Homebrew, apt, dnf, pacman, zypper, or apk, then rerun this command."
            ;;
    esac
}

install_for() {
    local dependency=$1
    if [[ "$SKIP_DEPS" -eq 1 ]]; then
        fail "$dependency is missing while --skip-deps is set. Install $dependency manually, then rerun with --skip-deps."
    fi
    case "$dependency:$PACKAGE_MANAGER" in
        go:brew) install_packages go ;;
        go:apt-get) install_linux_runtime go ;;
        go:dnf) install_packages golang ;;
        go:pacman) install_packages go ;;
        go:zypper) install_packages go ;;
        go:apk) install_packages go ;;
        node:brew) install_packages node ;;
        node:apt-get) install_linux_runtime node ;;
        node:dnf) install_packages nodejs npm ;;
        node:pacman) install_packages nodejs npm ;;
        node:zypper) install_packages nodejs npm ;;
        node:apk) install_packages nodejs npm ;;
        openssl:brew) install_packages openssl@3 ;;
        openssl:apt-get|openssl:dnf|openssl:pacman|openssl:zypper|openssl:apk) install_packages openssl ;;
        tesseract:brew) install_packages tesseract ;;
        tesseract:apt-get) install_packages tesseract-ocr ;;
        tesseract:dnf) install_packages tesseract ;;
        tesseract:pacman) install_packages tesseract ;;
        tesseract:zypper) install_packages tesseract-ocr ;;
        tesseract:apk) install_packages tesseract-ocr ;;
        ara:brew) install_packages tesseract-lang ;;
        ara:apt-get) install_packages tesseract-ocr-ara ;;
        ara:dnf) install_packages tesseract-langpack-ara ;;
        ara:pacman) install_packages tesseract-data-ara ;;
        ara:zypper) install_packages tesseract-ocr-traineddata-arabic ;;
        ara:apk) install_packages tesseract-ocr-data-ara ;;
        poppler:brew) install_packages poppler ;;
        poppler:apt-get|poppler:dnf|poppler:apk) install_packages poppler-utils ;;
        poppler:pacman|poppler:zypper) install_packages poppler ;;
        postgresql:brew) install_packages postgresql@17 ;;
        postgresql:apt-get) install_postgresql_apt ;;
        postgresql:dnf) install_packages postgresql17 postgresql17-server ;;
        postgresql:pacman) install_packages postgresql ;;
        postgresql:zypper) install_packages postgresql17 postgresql17-server ;;
        postgresql:apk) install_packages postgresql17 postgresql17-openrc ;;
        docker:brew)
            if ! brew list --cask docker >/dev/null 2>&1; then
                if ! brew install --cask docker; then
                    fail "Homebrew could not install Docker Desktop. Run 'brew install --cask docker', resolve the reported problem, then rerun."
                fi
            fi
            ;;
        docker:apt-get) install_packages docker.io docker-compose-v2 ;;
        docker:dnf) install_packages docker docker-compose-plugin ;;
        docker:pacman) install_packages docker docker-compose ;;
        docker:zypper) install_packages docker docker-compose ;;
        docker:apk) install_packages docker docker-cli-compose ;;
        compose:brew)
            if ! brew list --cask docker >/dev/null 2>&1; then
                if ! brew install --cask docker; then
                    fail "Homebrew could not install Docker Desktop. Run 'brew install --cask docker', resolve the reported problem, then rerun."
                fi
            fi
            ;;
        compose:apt-get) install_packages docker-compose-v2 ;;
        compose:dnf) install_packages docker-compose-plugin ;;
        compose:pacman|compose:zypper) install_packages docker-compose ;;
        compose:apk) install_packages docker-cli-compose ;;
        *) fail "No package mapping exists for $dependency with $PACKAGE_MANAGER. Install $dependency manually, then rerun with --skip-deps." ;;
    esac
}

# The version-17 server is not in the Ubuntu archives (jammy ships 14, noble
# ships 16), so a plain install would fail on every Ubuntu. When the archive
# cannot offer PostgreSQL 17, add the official PostgreSQL apt repository (PGDG)
# for this release exactly as documented on postgresql.org, then install from it.
install_postgresql_apt() {
    local candidate codename arch package=${1:-postgresql-17}
    if [[ "$APT_UPDATED" -eq 0 ]]; then
        run_privileged apt-get update
        APT_UPDATED=1
    fi
    candidate="$(apt_candidate "$package")"
    if [[ -n "$candidate" && "$candidate" != "(none)" ]]; then
        install_packages "$package"
        return
    fi
    codename="$(sed -n 's/^VERSION_CODENAME=//p' /etc/os-release 2>/dev/null | tr -d '"' | sed -n '1p')"
    [[ -n "$codename" ]] || fail "The Debian or Ubuntu release codename could not be read from /etc/os-release. Install PostgreSQL 17 from the official PostgreSQL apt repository yourself, then rerun with --db=native --skip-deps."
    case "$(uname -m)" in
        x86_64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) fail "The PostgreSQL apt repository offers no packages for $(uname -m). Install PostgreSQL 17 for this architecture yourself, then rerun with --db=native --skip-deps." ;;
    esac
    printf 'PostgreSQL 17 is not in the %s archive; adding the official PostgreSQL apt repository (PGDG).\n' "$codename"
    install_packages curl ca-certificates
    run_privileged install -d -m 0755 /usr/share/postgresql-common/pgdg
    # run_privileged stops the script on failure; --show-error keeps curl's own
    # network diagnosis visible above the generic privileged-command message.
    run_privileged curl --fail --silent --show-error --location \
        --output /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
        https://www.postgresql.org/media/keys/ACCC4CF8.asc
    printf 'Types: deb deb-src\nURIs: https://apt.postgresql.org/pub/repos/apt\nSuites: %s-pgdg\nArchitectures: %s\nComponents: main\nSigned-By: /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc\n' \
        "$codename" "$arch" \
        | run_privileged tee /etc/apt/sources.list.d/pgdg.sources >/dev/null
    run_privileged apt-get update
    candidate="$(apt_candidate "$package")"
    [[ -n "$candidate" && "$candidate" != "(none)" ]] || fail "PostgreSQL 17 is offered for neither the $codename archive nor its PostgreSQL apt repository. Install PostgreSQL 17 yourself, then rerun with --db=native --skip-deps."
    install_packages "$package"
}

# Install into a fresh version directory: overlaying a Go tree can leave stale files.
install_linux_runtime() (
    local runtime=$1 arch manifest archive checksum url staging toolchains destination
    install_packages curl ca-certificates python3 tar xz-utils
    case "$(uname -m)" in
        x86_64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) fail "Automatic runtime downloads support Linux x86_64 and arm64. Install Go and Node.js manually and use --skip-deps." ;;
    esac
    toolchains="${XDG_DATA_HOME:-$HOME/.local/share}/pms/toolchains"
    mkdir -p "$toolchains"
    staging="$(mktemp -d "$toolchains/.download.XXXXXX")"
    trap 'rm -rf "$staging"' EXIT
    if [[ "$runtime" == "go" ]]; then
        curl -fsSL 'https://go.dev/dl/?mode=json' -o "$staging/releases.json"
        manifest="$(python3 -c '
import json, sys
for release in json.load(open(sys.argv[1])):
    if release["stable"] and release["version"].startswith("go1.27."):
        for f in release["files"]:
            if f["os"] == "linux" and f["arch"] == sys.argv[2] and f["kind"] == "archive":
                print(f["filename"], f["sha256"])
                sys.exit(0)
sys.exit("No stable Go 1.27 download found")
' "$staging/releases.json" "$arch")"
        read -r archive checksum <<<"$manifest"
        url="https://go.dev/dl/$archive"
    else
        [[ "$arch" != "amd64" ]] || arch=x64
        curl -fsSL https://nodejs.org/dist/latest-v24.x/SHASUMS256.txt -o "$staging/checksums"
        manifest="$(awk -v suffix="-linux-$arch.tar.xz" 'index($2, suffix) && substr($2, length($2)-length(suffix)+1) == suffix {print $2, $1}' "$staging/checksums")"
        read -r archive checksum <<<"$manifest"
        url="https://nodejs.org/dist/latest-v24.x/$archive"
    fi
    [[ "$archive" =~ ^[a-zA-Z0-9._-]+$ && "$checksum" =~ ^[a-f0-9]{64}$ ]] || fail "Invalid $runtime download metadata."
    destination="$toolchains/${archive%.tar.*}"
    if [[ ! -d "$destination" ]]; then
        curl -fsSL "$url" -o "$staging/$archive"
        (cd "$staging" && printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check --status) || fail "$runtime archive checksum mismatch."
        mkdir "$staging/unpacked"
        tar -xf "$staging/$archive" --strip-components=1 -C "$staging/unpacked"
        mv "$staging/unpacked" "$destination"
    fi
    [[ ! -e "$toolchains/$runtime" || -L "$toolchains/$runtime" ]] || fail "$toolchains/$runtime must be a setup-managed symlink."
    ln -sfn "$destination" "$toolchains/$runtime"
)

go_is_current() {
    local version major minor
    version="$(go version 2>/dev/null | sed -nE 's/.* go([0-9]+)\.([0-9]+).*/\1.\2/p')"
    major=${version%%.*}
    minor=${version#*.}
    [[ -n "$version" ]] && (( major > 1 || (major == 1 && minor >= 27) ))
}

node_is_current() {
    node -e '
        const [major, minor, patch] = process.versions.node.split(".").map(Number);
        process.exit((major === 22 && (minor > 22 || (minor === 22 && patch >= 3))) ||
            (major === 24 && minor >= 15) || major >= 26 ? 0 : 1);
    ' 2>/dev/null
}

refresh_brew_path() {
    local prefix
    if [[ "$PACKAGE_MANAGER" == "brew" ]]; then
        prefix="$(brew --prefix)"
        export PATH="$prefix/bin:$prefix/sbin:$PATH"
        if brew list --formula openssl@3 >/dev/null 2>&1; then
            export PATH="$(brew --prefix openssl@3)/bin:$PATH"
        fi
        if brew list --formula postgresql@17 >/dev/null 2>&1; then
            export PATH="$(brew --prefix postgresql@17)/bin:$PATH"
        fi
    fi
}

openssl_is_suitable() {
    local help_text
    command_exists openssl || return 1
    help_text="$(openssl req -help 2>&1)" || return 1
    [[ "$help_text" == *"-addext"* ]]
}

tesseract_has_arabic() {
    local languages
    command_exists tesseract || return 1
    languages="$(tesseract --list-langs 2>/dev/null)" || return 1
    grep -qx 'ara' <<<"$languages"
}

refresh_postgresql_path() {
    local candidate
    refresh_brew_path
    for candidate in /usr/lib/postgresql/17/bin /usr/pgsql-17/bin /usr/lib/postgresql17/bin /usr/libexec/postgresql17; do
        if [[ -x "$candidate/postgres" ]]; then
            export PATH="$candidate:$PATH"
            return
        fi
    done
}

postgres_binary_major() {
    command_exists postgres || return 1
    postgres --version 2>/dev/null | sed -nE 's/.* ([0-9]+)(\.[0-9]+)+.*/\1/p'
}

native_server_reachable() {
    refresh_postgresql_path
    command_exists postgres && command_exists pg_isready && pg_isready --timeout=1 >/dev/null 2>&1
}

select_database_mode() {
    if [[ -n "$DB_REQUEST" ]]; then
        DATABASE_MODE="$DB_REQUEST"
        printf 'Database mode: %s (selected by --db=%s).\n' "$DATABASE_MODE" "$DATABASE_MODE"
    elif native_server_reachable; then
        DATABASE_MODE="native"
        printf 'Database mode: native (a local PostgreSQL server is installed and reachable).\n'
    else
        DATABASE_MODE="docker"
        printf 'Database mode: docker (no installed native PostgreSQL server was reachable).\n'
    fi
}

ensure_pacman_offers_postgresql_17() {
    local offered_version offered_major
    offered_version="$(pacman -Si postgresql 2>/dev/null | sed -nE 's/^Version[[:space:]]*:[[:space:]]*([^[:space:]]+).*/\1/p' | sed -n '1p')"
    offered_major=${offered_version%%.*}
    if [[ -n "$offered_version" && "$offered_major" != "17" ]]; then
        fail "The configured pacman repository offers PostgreSQL $offered_version, not 17. Install PostgreSQL 17 from a maintained package source, then rerun with --db=native --skip-deps; this installer will not upgrade or downgrade the server."
    fi
}

ensure_postgresql_installed() {
    local installed_major
    refresh_postgresql_path
    installed_major="$(postgres_binary_major || true)"
    if [[ -n "$installed_major" && "$installed_major" != "17" ]]; then
        fail "PostgreSQL server major version $installed_major is installed; version 17 is required. Remove it or migrate it to PostgreSQL 17 yourself, then rerun. This installer will not upgrade or downgrade PostgreSQL."
    fi
    if [[ "$SKIP_DEPS" -eq 0 || -z "$installed_major" ]]; then
        [[ "$PACKAGE_MANAGER" != "pacman" ]] || ensure_pacman_offers_postgresql_17
        install_for postgresql
        refresh_postgresql_path
        installed_major="$(postgres_binary_major || true)"
    fi
    [[ "$installed_major" == "17" ]] || fail "PostgreSQL 17 was not found after installation. Install its server and client binaries, ensure they are on PATH, then rerun with --db=native --skip-deps."
    command_exists psql && command_exists pg_isready || fail "PostgreSQL 17 is incomplete. Ensure psql and pg_isready are installed and on PATH, then rerun with --db=native --skip-deps."
}

ensure_dependencies() {
    if [[ "$SKIP_DEPS" -eq 0 ]]; then
        install_for node
        install_for go
        refresh_brew_path
    fi
    command_exists go && go_is_current || fail "Go 1.27 or newer is required. Rerun without --skip-deps to install it."
    command_exists node && command_exists npm && node_is_current || fail "Angular requires Node.js 22.22.3+, 24.15.0+, or 26+. Rerun without --skip-deps to install a supported version."

    if [[ "$SKIP_DEPS" -eq 0 ]] || ! openssl_is_suitable; then
        install_for openssl
        refresh_brew_path
    fi
    openssl_is_suitable || fail "OpenSSL with 'req -addext' support is required. Install OpenSSL 3, ensure 'openssl' is on PATH, then rerun with --skip-deps."

    if [[ "$SKIP_DEPS" -eq 0 ]] || ! command_exists tesseract; then
        install_for tesseract
        refresh_brew_path
    fi
    command_exists tesseract || fail "Tesseract is required. Install it, ensure 'tesseract' is on PATH, then rerun with --skip-deps."
    if [[ "$SKIP_DEPS" -eq 0 ]] || ! tesseract_has_arabic; then
        install_for ara
    fi
    if ! tesseract_has_arabic; then
        fail "Tesseract Arabic data is missing. Install the 'ara' trained data for your Tesseract installation, verify 'tesseract --list-langs' lists ara, then rerun with --skip-deps."
    fi

    if [[ "$SKIP_DEPS" -eq 0 ]] || ! command_exists pdftotext || ! command_exists pdftoppm || ! command_exists pdfinfo; then
        install_for poppler
        refresh_brew_path
    fi
    if ! command_exists pdftotext || ! command_exists pdftoppm || ! command_exists pdfinfo; then
        fail "Poppler is incomplete. Install pdftotext, pdftoppm, and pdfinfo, ensure all three are on PATH, then rerun with --skip-deps."
    fi

    if [[ "$DATABASE_MODE" == "native" ]]; then
        ensure_postgresql_installed
    else
        if [[ "$SKIP_DEPS" -eq 0 ]] || ! command_exists docker; then
            install_for docker
            refresh_brew_path
        fi
        command_exists docker || fail "Docker was installed but is not on PATH. Restart the shell, then rerun this command."
        if [[ "$SKIP_DEPS" -eq 0 ]] || ! docker compose version >/dev/null 2>&1; then
            install_for compose
            refresh_brew_path
        fi
        docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required. Install the Compose v2 plugin, then rerun this command."
    fi
}

ensure_docker_running() {
    if docker info >/dev/null 2>&1; then
        return
    fi
    if [[ "$OS_NAME" == "macOS" ]]; then
        if [[ ! -d /Applications/Docker.app && ! -d "$HOME/Applications/Docker.app" ]]; then
            install_for docker
        fi
        if ! open -a Docker >/dev/null 2>&1; then
            fail "Docker Desktop is installed but could not be opened. Start Docker Desktop manually, wait until it is ready, then rerun this command."
        fi
    elif command_exists systemctl; then
        run_privileged systemctl enable --now docker
    elif command_exists rc-service; then
        run_privileged rc-update add docker default
        run_privileged rc-service docker start
    else
        fail "Docker is installed but its daemon is not running. Start the Docker service, then rerun this command."
    fi

    local attempt
    for ((attempt = 1; attempt <= 60; attempt++)); do
        if docker info >/dev/null 2>&1; then
            return
        fi
        if [[ "$OS_NAME" == "Linux" && "$(id -u)" -ne 0 ]] && sudo -n docker info >/dev/null 2>&1; then
            DOCKER_CMD=(sudo -n --preserve-env=POSTGRES_PASSWORD docker)
            return
        fi
        sleep 1
    done
    if [[ "$OS_NAME" == "Linux" ]]; then
        fail "Docker did not become ready within 60 seconds. Verify 'docker info' succeeds for this account; if it fails with a socket permission error, add this account to the docker group with 'sudo usermod -aG docker $USER' and sign in again, or grant this account passwordless sudo for docker, then rerun this command."
    fi
    fail "Docker did not become ready within 60 seconds. Start Docker Desktop, verify 'docker info' succeeds, then rerun this command."
}

quote_env_value() {
    local raw_value=$1
    raw_value=${raw_value//\'/\'\\\'\'}
    printf "'%s'" "$raw_value"
}

load_env() {
    local previous_dir=$PWD
    cd "$ROOT"
    unset PMS_KEK PMS_DATABASE_OWNER_URL PMS_DATABASE_APP_URL PMS_TLS_CERT_PATH PMS_TLS_KEY_PATH
    unset PMS_LISTEN_ADDR PMS_ATTACHMENT_STORE PMS_ADMIN_PASSWORD PMS_WEB_DIST PMS_BACKUP_DIR SEED_USERNAME
    set +u
    set -a
    # shellcheck disable=SC1090 -- the repository-root .env path is resolved at runtime.
    if ! . "$ENV_FILE"; then
        set +a
        set -u
        fail "Could not read $ENV_FILE as a shell environment file. Fix its syntax, then rerun this command."
    fi
    set +a
    set -u
    cd "$previous_dir"
}

set_env_if_empty() {
    local name=$1 env_value=$2
    if [[ -n "${!name-}" ]]; then
        return
    fi
    printf '%s=%s\n' "$name" "$(quote_env_value "$env_value")" >>"$ENV_FILE"
    printf -v "$name" '%s' "$env_value"
    export "$name"
}

rewrite_env_value() {
    local name=$1 env_value=$2 line rewritten=""
    while IFS= read -r line || [[ -n "$line" ]]; do
        if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?${name}[[:space:]]*= ]]; then
            line="$name=$(quote_env_value "$env_value")"
        fi
        printf -v rewritten '%s%s\n' "$rewritten" "$line"
    done <"$ENV_FILE"
    printf '%s' "$rewritten" >"$ENV_FILE"
    printf -v "$name" '%s' "$env_value"
    export "$name"
    chmod 600 "$ENV_FILE"
}

dsn_password_component() {
    local dsn=$1 authority user_info
    authority=${dsn#*://}
    user_info=${authority%%@*}
    [[ "$user_info" == *:* ]] || return 1
    printf '%s' "${user_info#*:}"
}

decode_url_component() {
    node -e 'let s=""; process.stdin.on("data", c => s += c); process.stdin.on("end", () => process.stdout.write(decodeURIComponent(s)));'
}

prepare_database_dsn() {
    local env_name=$1 username=$2 default_password=$3 current_dsn encoded_password generated_password decoded_password
    current_dsn=${!env_name-}
    DATABASE_PASSWORD_ACTION="keep"
    if [[ -z "$current_dsn" ]]; then
        generated_password="$(openssl rand -hex 24)" || fail "OpenSSL could not generate the $username database password. Repair OpenSSL, then rerun this command."
        [[ ${#generated_password} -eq 48 ]] || fail "OpenSSL returned an invalid $username database password. Repair OpenSSL, then rerun this command."
        current_dsn="postgres://$username:$generated_password@127.0.0.1:5432/pms?sslmode=disable"
        set_env_if_empty "$env_name" "$current_dsn"
        DATABASE_PASSWORD_ACTION="fresh"
    fi
    encoded_password="$(dsn_password_component "$current_dsn" || true)"
    if ! decoded_password="$(printf '%s' "$encoded_password" | decode_url_component 2>/dev/null)"; then
        fail "$env_name has an invalid percent-encoded password component. Correct that DSN in .env, then rerun."
    fi
    DATABASE_TARGET_PASSWORD="$decoded_password"
    DATABASE_TARGET_URL="$current_dsn"
    if [[ "$encoded_password" == "$default_password" ]]; then
        generated_password="$(openssl rand -hex 24)" || fail "OpenSSL could not generate the $username database password. Repair OpenSSL, then rerun this command."
        DATABASE_TARGET_PASSWORD="$generated_password"
        DATABASE_TARGET_URL="postgres://$username:$generated_password@127.0.0.1:5432/pms?sslmode=disable"
        DATABASE_PASSWORD_ACTION="rotate"
    fi
}

configure_env() {
    if [[ ! -f "$ENV_FILE" ]]; then
        cp "$ROOT/.env.example" "$ENV_FILE"
    fi
    chmod 600 "$ENV_FILE"
    load_env

    if [[ -z "${PMS_KEK-}" ]]; then
        local generated_kek
        generated_kek="$(openssl rand -base64 32)" || fail "OpenSSL could not generate PMS_KEK. Repair OpenSSL, then rerun this command."
        set_env_if_empty PMS_KEK "$generated_kek"
    fi
    prepare_database_dsn PMS_DATABASE_OWNER_URL pms_owner dev_only_owner_pw
    OWNER_PASSWORD_ACTION="$DATABASE_PASSWORD_ACTION"
    OWNER_TARGET_PASSWORD="$DATABASE_TARGET_PASSWORD"
    OWNER_TARGET_URL="$DATABASE_TARGET_URL"
    if [[ "$OWNER_PASSWORD_ACTION" == "rotate" ]]; then
        OWNER_STARTUP_PASSWORD="dev_only_owner_pw"
    else
        OWNER_STARTUP_PASSWORD="$OWNER_TARGET_PASSWORD"
    fi
    prepare_database_dsn PMS_DATABASE_APP_URL pms_app dev_only_app_pw
    APP_PASSWORD_ACTION="$DATABASE_PASSWORD_ACTION"
    APP_TARGET_PASSWORD="$DATABASE_TARGET_PASSWORD"
    APP_TARGET_URL="$DATABASE_TARGET_URL"
    set_env_if_empty PMS_TLS_CERT_PATH "$CERT_FILE"
    set_env_if_empty PMS_TLS_KEY_PATH "$KEY_FILE"
    set_env_if_empty PMS_LISTEN_ADDR "0.0.0.0:8443"
    set_env_if_empty PMS_ATTACHMENT_STORE "${XDG_DATA_HOME:-$HOME/.local/share}/pms/attachments"
    set_env_if_empty PMS_WEB_DIST "$WEB_DIST"
    if [[ -z "${PMS_ADMIN_PASSWORD-}" ]]; then
        local generated_admin_password
        generated_admin_password="$(openssl rand -base64 18)" || fail "OpenSSL could not generate the administrator password. Repair OpenSSL, then rerun this command."
        [[ ${#generated_admin_password} -eq 24 ]] || fail "OpenSSL returned an invalid administrator password. Repair OpenSSL, then rerun this command."
        set_env_if_empty PMS_ADMIN_PASSWORD "$generated_admin_password"
        ADMIN_PASSWORD_GENERATED=1
    fi

    [[ "$PMS_ATTACHMENT_STORE" == /* ]] || fail "PMS_ATTACHMENT_STORE must be absolute. Set an absolute directory outside the repository in .env, then rerun."
    mkdir -p "$PMS_ATTACHMENT_STORE"
    [[ -d "$PMS_ATTACHMENT_STORE" ]] || fail "PMS_ATTACHMENT_STORE is not a directory. Set it to an absolute directory outside the repository in .env, then rerun."
    local store_path decoded_kek
    store_path="$(cd "$PMS_ATTACHMENT_STORE" && pwd -P)"
    case "$store_path" in
        "$ROOT"|"$ROOT"/*) fail "PMS_ATTACHMENT_STORE is inside the repository. Set it to an absolute directory outside $ROOT in .env, then rerun." ;;
    esac
    [[ "$PMS_WEB_DIST" == /* ]] || fail "PMS_WEB_DIST must be absolute. Set it to the Angular production output directory in .env, then rerun."
    if [[ "$PMS_WEB_DIST" != "$WEB_DIST" && ! -d "$PMS_WEB_DIST" ]]; then
        fail "PMS_WEB_DIST does not exist and is not the bundle this setup builds. Correct it in .env, then rerun."
    fi
    if [[ -d "$PMS_WEB_DIST" ]]; then
        local web_dist_path
        web_dist_path="$(cd "$PMS_WEB_DIST" && pwd -P)"
        case "$store_path" in
            "$web_dist_path"|"$web_dist_path"/*) fail "PMS_WEB_DIST and PMS_ATTACHMENT_STORE overlap. Set them to separate directories in .env, then rerun." ;;
        esac
        case "$web_dist_path" in
            "$store_path"|"$store_path"/*) fail "PMS_WEB_DIST and PMS_ATTACHMENT_STORE overlap. Set them to separate directories in .env, then rerun." ;;
        esac
    fi
    decoded_kek="$(printf '%s' "$PMS_KEK" | openssl base64 -d -A 2>/dev/null | wc -c | tr -d '[:space:]' || true)"
    [[ "$decoded_kek" == "32" ]] || fail "PMS_KEK must be base64 for exactly 32 bytes. Restore the correct key in .env; do not generate a replacement for existing encrypted data."
    [[ ${#PMS_ADMIN_PASSWORD} -ge 12 ]] || fail "PMS_ADMIN_PASSWORD must be at least 12 characters. Set a longer value in .env, then rerun."
    listen_port >/dev/null
    # A narrower bind is a legitimate operator choice for a local-only install,
    # so it is a warning rather than a refusal; the summary then omits the LAN
    # URL instead of printing one that cannot be reached.
    case "$PMS_LISTEN_ADDR" in
        :*|0.0.0.0:*|\[::\]:*) LISTEN_ALL_INTERFACES=1 ;;
        *)
            LISTEN_ALL_INTERFACES=0
            printf 'Note: PMS_LISTEN_ADDR is %s, so only this machine can reach the server. Set it to 0.0.0.0:%s in .env for LAN access.\n' \
                "$PMS_LISTEN_ADDR" "${PMS_LISTEN_ADDR##*:}"
            ;;
    esac
    chmod 700 "$PMS_ATTACHMENT_STORE"
    chmod 600 "$ENV_FILE"
}

ensure_tls() {
    mkdir -p "$CERT_DIR"
    if [[ -f "$CERT_FILE" && -f "$KEY_FILE" ]]; then
        printf 'Reusing existing TLS certificate.\n'
    elif [[ -e "$CERT_FILE" || -e "$KEY_FILE" ]]; then
        fail "Only one of $CERT_FILE and $KEY_FILE exists. Restore the missing matching file or move the remaining file aside, then rerun."
    else
        if ! openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 \
            -keyout "$KEY_FILE" \
            -out "$CERT_FILE" \
            -subj "/CN=localhost" \
            -addext "subjectAltName=DNS:localhost,DNS:$HOST_NAME,IP:127.0.0.1,IP:$LAN_IP" >/dev/null 2>&1; then
            fail "OpenSSL could not create the TLS certificate. Run 'openssl version', repair OpenSSL, then rerun this command."
        fi
        chmod 600 "$KEY_FILE"
        printf 'Created a self-signed TLS certificate for localhost, %s, and %s.\n' "$HOST_NAME" "$LAN_IP"
    fi

    [[ -f "$PMS_TLS_CERT_PATH" ]] || fail "PMS_TLS_CERT_PATH does not exist. Put the certificate at the path configured in .env, then rerun."
    [[ -f "$PMS_TLS_KEY_PATH" ]] || fail "PMS_TLS_KEY_PATH does not exist. Put the matching private key at the path configured in .env, then rerun."
}

docker_compose() {
    POSTGRES_PASSWORD="$OWNER_STARTUP_PASSWORD" "${DOCKER_CMD[@]}" compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

native_psql() {
    if [[ "$OS_NAME" == "macOS" ]]; then
        psql -X --dbname postgres -v ON_ERROR_STOP=1 "$@"
    elif [[ "$(id -u)" -eq 0 ]]; then
        runuser -u postgres -- psql -X --dbname postgres -v ON_ERROR_STOP=1 "$@"
    else
        sudo -n -u postgres psql -X --dbname postgres -v ON_ERROR_STOP=1 "$@"
    fi
}

initialize_native_cluster() {
    case "$PACKAGE_MANAGER" in
        dnf)
            [[ -f /var/lib/pgsql/17/data/PG_VERSION ]] || run_privileged /usr/pgsql-17/bin/postgresql-17-setup initdb
            ;;
        pacman)
            if [[ ! -f /var/lib/postgres/data/PG_VERSION ]]; then
                run_privileged install -d -o postgres -g postgres /var/lib/postgres/data
                run_privileged runuser -u postgres -- initdb -D /var/lib/postgres/data
            fi
            ;;
        zypper)
            if [[ ! -f /var/lib/pgsql/data/PG_VERSION ]] && command_exists postgresql-setup; then
                run_privileged postgresql-setup --initdb
            fi
            ;;
        apk)
            [[ -f /var/lib/postgresql/17/data/PG_VERSION ]] || run_privileged rc-service postgresql setup
            ;;
    esac
}

start_native_service() {
    if [[ "$OS_NAME" == "macOS" ]]; then
        brew services start postgresql@17 || fail "PostgreSQL 17 could not be started. Run 'brew services start postgresql@17', resolve the reported problem, then rerun."
        return
    fi
    initialize_native_cluster
    case "$PACKAGE_MANAGER" in
        apt-get) run_privileged systemctl enable --now postgresql ;;
        dnf) run_privileged systemctl enable --now postgresql-17 ;;
        pacman|zypper) run_privileged systemctl enable --now postgresql ;;
        apk)
            run_privileged rc-update add postgresql default
            run_privileged rc-service postgresql start
            ;;
        *) fail "PostgreSQL 17 is installed, but its service manager is unsupported. Start and enable PostgreSQL 17 at boot, then rerun with --db=native --skip-deps." ;;
    esac
}

restart_native_service() {
    if [[ "$OS_NAME" == "macOS" ]]; then
        brew services restart postgresql@17 || fail "PostgreSQL 17 could not be restarted. Run 'brew services restart postgresql@17', resolve the reported problem, then rerun."
        return
    fi
    case "$PACKAGE_MANAGER" in
        apt-get|pacman|zypper) run_privileged systemctl restart postgresql ;;
        dnf) run_privileged systemctl restart postgresql-17 ;;
        apk) run_privileged rc-service postgresql restart ;;
    esac
}

native_server_major() {
    native_psql -Atqc 'SHOW server_version' 2>/dev/null | sed -nE 's/^([0-9]+).*/\1/p'
}

require_native_superuser_access() {
    if native_psql -Atqc 'SELECT 1' >/dev/null 2>&1; then
        return
    fi
    if [[ "$OS_NAME" == "macOS" ]]; then
        fail "PostgreSQL is running, but the current macOS user cannot open its local postgres database. Run 'psql postgres', restore local Homebrew superuser access, then rerun with --db=native."
    fi
    fail "PostgreSQL is running, but passwordless bootstrap access failed. Run 'sudo -u postgres psql postgres' and repair local peer access, then run 'sudo -v' and rerun with --db=native."
}

configure_native_listener() {
    printf "%s\n" "ALTER SYSTEM SET listen_addresses TO '127.0.0.1';" "ALTER SYSTEM SET port TO '5432';" | native_psql >/dev/null
    restart_native_service
}

ensure_native_owner_role() {
    local role_exists
    role_exists="$(native_psql -Atqc "SELECT 1 FROM pg_roles WHERE rolname = 'pms_owner'" 2>/dev/null)"
    if [[ -z "$role_exists" ]]; then
        printf 'CREATE ROLE pms_owner LOGIN SUPERUSER PASSWORD %s;\n' "$(sql_password_literal "$OWNER_STARTUP_PASSWORD")" \
            | native_psql >/dev/null 2>&1 || fail "Could not create the database owner with the configured password."
    else
        printf '%s\n' 'ALTER ROLE pms_owner LOGIN SUPERUSER;' | native_psql >/dev/null
        if [[ "$OWNER_PASSWORD_ACTION" == "fresh" ]]; then
            set_database_role_password pms_owner "$OWNER_TARGET_PASSWORD"
        fi
    fi
}

ensure_native_database() {
    local database_exists
    ensure_native_owner_role
    database_exists="$(native_psql -Atqc "SELECT 1 FROM pg_database WHERE datname = 'pms'" 2>/dev/null)"
    if [[ -z "$database_exists" ]]; then
        printf '%s\n' 'CREATE DATABASE pms OWNER pms_owner;' | native_psql >/dev/null
    else
        printf '%s\n' 'ALTER DATABASE pms OWNER TO pms_owner;' | native_psql >/dev/null
    fi
}

start_database() {
    if [[ "$DATABASE_MODE" == "docker" ]]; then
        if [[ "$SKIP_DEPS" -eq 0 ]]; then
            docker_compose pull postgres || fail "Could not update the PostgreSQL 17 image."
        fi
        docker_compose up -d || fail "PostgreSQL could not be started. Run 'docker compose --env-file \"$ENV_FILE\" -f \"$COMPOSE_FILE\" up -d', fix the reported problem, then rerun."
        if [[ "$OWNER_PASSWORD_ACTION" == "fresh" ]]; then
            set_database_role_password pms_owner "$OWNER_TARGET_PASSWORD"
        fi
        return
    fi
    start_native_service
    require_native_superuser_access
    local server_major
    server_major="$(native_server_major)"
    [[ "$server_major" == "17" ]] || fail "The reachable native PostgreSQL server is major version ${server_major:-unknown}; version 17 is required. Stop that server and make PostgreSQL 17 the local server, then rerun. Setup will not upgrade or downgrade it."
    configure_native_listener
    require_native_superuser_access
    ensure_native_database
}

database_is_ready() {
    if [[ "$DATABASE_MODE" == "docker" ]]; then
        docker_compose exec -T postgres pg_isready -U pms_owner -d pms >/dev/null 2>&1
    else
        pg_isready --host 127.0.0.1 --port 5432 --dbname pms >/dev/null 2>&1
    fi
}

run_migrations() {
    local attempt
    for ((attempt = 1; attempt <= 30; attempt++)); do
        if database_is_ready; then
            ensure_application_role
            if ! (
                cd "$ROOT/api"
                unset PMS_KEK PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD
                go run ./cmd/migrate up
            ); then
                fail "Migrations failed. Load .env, run 'cd api && go run ./cmd/migrate up', fix the reported problem, then rerun."
            fi
            return
        fi
        sleep 2
    done
    if [[ "$DATABASE_MODE" == "docker" ]]; then
        fail "PostgreSQL did not become ready within 60 seconds. Run 'docker compose --env-file \"$ENV_FILE\" -f \"$COMPOSE_FILE\" logs postgres', fix the reported problem, then rerun this command."
    fi
    fail "Native PostgreSQL did not become ready on 127.0.0.1:5432 within 60 seconds. Inspect its service status and logs, then rerun with --db=native."
}

database_super_psql() {
    if [[ "$DATABASE_MODE" == "docker" ]]; then
        docker_compose exec -T postgres psql -X --username pms_owner --dbname postgres -v ON_ERROR_STOP=1 -At
    else
        native_psql -At
    fi
}

# E-string quoting preserves quotes and backslashes regardless of server settings.
sql_password_literal() {
    local password=$1
    password=${password//\\/\\\\}
    password=${password//\'/\'\'}
    printf "E'%s'" "$password"
}

ensure_application_role() {
    local role_exists
    role_exists="$(printf '%s\n' "SELECT 1 FROM pg_roles WHERE rolname = 'pms_app';" | database_super_psql)"
    if [[ -z "$role_exists" ]]; then
        printf 'CREATE ROLE pms_app LOGIN PASSWORD %s;\n' "$(sql_password_literal "$APP_TARGET_PASSWORD")" \
            | database_super_psql >/dev/null 2>&1 || fail "Could not create the application database role with the configured password."
    fi
}

set_database_role_password() {
    local role=$1 password=$2
    printf 'ALTER ROLE %s PASSWORD %s;\n' "$role" "$(sql_password_literal "$password")" \
        | database_super_psql >/dev/null 2>&1 || fail "Could not set the configured password for $role."
}

converge_database_passwords() {
    if [[ "$OWNER_PASSWORD_ACTION" != "keep" ]]; then
        set_database_role_password pms_owner "$OWNER_TARGET_PASSWORD"
        if [[ "$OWNER_PASSWORD_ACTION" == "rotate" ]]; then
            rewrite_env_value PMS_DATABASE_OWNER_URL "$OWNER_TARGET_URL"
            printf 'Rotated the default pms_owner database password.\n'
        fi
    fi
    if [[ "$APP_PASSWORD_ACTION" != "keep" ]]; then
        set_database_role_password pms_app "$APP_TARGET_PASSWORD"
        if [[ "$APP_PASSWORD_ACTION" == "rotate" ]]; then
            rewrite_env_value PMS_DATABASE_APP_URL "$APP_TARGET_URL"
            printf 'Rotated the default pms_app database password.\n'
        fi
    fi
}

seed_administrator() {
    local seed_output
    if ! seed_output="$(
        cd "$ROOT/api"
        unset PMS_KEK PMS_DATABASE_APP_URL DEV_ADMIN_PASSWORD
        go run ./cmd/admintool seed-admin --username "${SEED_USERNAME:-admin}" --password-env PMS_ADMIN_PASSWORD 2>&1
    )"; then
        if [[ "$seed_output" != *"an account with this canonical username already exists"* ]]; then
            fail "The administrator could not be created. Load .env and run 'cd api && go run ./cmd/admintool seed-admin --username ${SEED_USERNAME:-admin} --password-env PMS_ADMIN_PASSWORD', fix the reported problem, then rerun."
        fi
    fi
    # A newly created account carries must_change_password = true (the account
    # table's default), which would make the .env password unusable until the
    # operator changed it interactively. Clearing it here is what makes the
    # promise of this script true: one command, then sign in with the password
    # already in .env. The same call reconciles a pre-existing account, so both
    # paths end in the same state.
    if ! (
        cd "$ROOT/api"
        unset PMS_KEK PMS_DATABASE_APP_URL DEV_ADMIN_PASSWORD
        go run ./cmd/admintool reset-admin --username "${SEED_USERNAME:-admin}" --password-env PMS_ADMIN_PASSWORD --must-change=false
    ) >/dev/null 2>&1; then
        fail "The administrator password could not be set. Load .env and run 'cd api && go run ./cmd/admintool reset-admin --username ${SEED_USERNAME:-admin} --password-env PMS_ADMIN_PASSWORD --must-change=false', then rerun."
    fi
    if [[ "$ADMIN_PASSWORD_GENERATED" -eq 1 ]]; then
        printf 'The generated administrator password was written to .env.\n'
    fi
}

build_application() {
    if [[ -f "$ROOT/web/package-lock.json" ]]; then
        (unset PMS_KEK PMS_DATABASE_OWNER_URL PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD; cd "$ROOT/web" && npm ci) || fail "Web dependencies could not be installed. Resolve the npm error, then rerun this command."
    else
        (unset PMS_KEK PMS_DATABASE_OWNER_URL PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD; cd "$ROOT/web" && npm install) || fail "Web dependencies could not be installed. Resolve the npm error, then rerun this command."
    fi
    (unset PMS_KEK PMS_DATABASE_OWNER_URL PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD; cd "$ROOT/web" && npm run build -- --configuration production) || fail "The production web build failed. Resolve the npm error, then rerun this command."
    mkdir -p "$ROOT/api/bin"
    (unset PMS_KEK PMS_DATABASE_OWNER_URL PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD; cd "$ROOT/api" && go build -o "$ROOT/api/bin/pms-api" ./cmd/server) || fail "The API build failed. Resolve the Go error, then rerun this command."
}

stop_managed_server() {
    local pid_file=$1 pid command_line
    # `return` with no argument returns the status of the last command — here the
    # failed test — which under `set -e` aborts the whole script silently. The
    # explicit 0 is what makes "no server was running" a success.
    [[ -f "$pid_file" ]] || return 0
    pid="$(tr -d '[:space:]' <"$pid_file")"
    if [[ ! "$pid" =~ ^[0-9]+$ ]] || ! kill -0 "$pid" 2>/dev/null; then
        rm -f "$pid_file"
        return
    fi
    command_line="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    if [[ "$command_line" != *"$ROOT/api/bin/pms-api"* ]]; then
        fail "The saved server PID $pid belongs to another process. Remove $pid_file only after verifying that process, then rerun."
    fi
    kill "$pid"
    local attempt
    for ((attempt = 1; attempt <= 10; attempt++)); do
        if ! kill -0 "$pid" 2>/dev/null; then
            rm -f "$pid_file"
            return
        fi
        sleep 1
    done
    fail "The existing PMS server did not stop. Stop PID $pid manually, then rerun this command."
}

listen_port() {
    local port=${PMS_LISTEN_ADDR##*:}
    port=${port%]}
    [[ "$port" =~ ^[0-9]+$ ]] && (( port >= 1 && port <= 65535 )) || fail "PMS_LISTEN_ADDR must end in a valid TCP port. Fix it in .env, then rerun."
    printf '%s' "$port"
}

port_owner() {
    local port=$1 owner=""
    if command_exists lsof && lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        owner="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -Fc 2>/dev/null | sed -n 's/^c//p' | sed -n '1p')"
        printf '%s' "${owner:-another process}"
        return 0
    fi
    if command_exists ss && [[ -n "$(ss -ltn "sport = :$port" 2>/dev/null | tail -n +2)" ]]; then
        printf '%s' 'another process'
        return 0
    fi
    return 1
}

health_ready() {
    local port=$1 status_line
    status_line="$(printf 'GET /api/v1/health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n' \
        | openssl s_client -connect "127.0.0.1:$port" -servername localhost -quiet 2>/dev/null \
        | sed -n '1p' || true)"
    [[ "$status_line" =~ ^HTTP/[0-9.]+\ 200\  ]]
}

start_application() {
    local state_dir pid_file log_file port owner pid attempt
    state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/pms"
    mkdir -p "$state_dir"
    chmod 700 "$state_dir"
    pid_file="$state_dir/pms-api.pid"
    log_file="$state_dir/pms-api.log"
    stop_managed_server "$pid_file"

    port="$(listen_port)"
    if owner="$(port_owner "$port")"; then
        fail "PMS_LISTEN_ADDR port $port is already in use by $owner. Stop that process or change PMS_LISTEN_ADDR in .env, then rerun."
    fi

    (
        cd "$ROOT/api"
        unset PMS_DATABASE_OWNER_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD
        exec nohup "$ROOT/api/bin/pms-api" >>"$log_file" 2>&1
    ) &
    pid=$!
    printf '%s\n' "$pid" >"$pid_file"
    chmod 600 "$pid_file" "$log_file"

    for ((attempt = 1; attempt <= 60; attempt++)); do
        if ! kill -0 "$pid" 2>/dev/null; then
            rm -f "$pid_file"
            fail "The PMS server exited during startup. Inspect $log_file, fix the reported problem, then rerun."
        fi
        if health_ready "$port"; then
            printf '\nSign in at: https://localhost:%s\n' "$port"
            if [[ "$LISTEN_ALL_INTERFACES" -eq 1 ]]; then
                printf 'LAN URL: https://%s:%s\n' "$LAN_IP" "$port"
            fi
            printf 'Administrator username: %s\n' "${SEED_USERNAME:-admin}"
            printf 'The administrator password is in .env.\n'
            return
        fi
        sleep 1
    done
    kill "$pid" 2>/dev/null || true
    rm -f "$pid_file"
    fail "The PMS server did not answer its health check within 60 seconds. Inspect $log_file, fix the reported problem, then rerun."
}

. "$SCRIPT_DIR/linux-services.sh"

heading "Preflight"
case "$(uname -s)" in
    Darwin) OS_NAME="macOS" ;;
    Linux) OS_NAME="Linux" ;;
    *) fail "This script supports macOS and Linux. On Windows, run scripts\\setup.ps1." ;;
esac
if [[ "$OS_NAME" == "Linux" ]]; then
    [[ "$(id -u)" -ne 0 ]] || fail "Run setup as the app user, not root: sudo -v, then bash scripts/setup.sh."
    command_exists systemctl && command_exists loginctl && [[ -d /run/systemd/system ]] || fail "Automatic startup and daily backups require systemd (for example Ubuntu or Debian)."
fi
HOST_NAME="$(hostname 2>/dev/null || true)"
[[ "$HOST_NAME" =~ ^[A-Za-z0-9.-]+$ ]] || fail "The machine hostname cannot be placed in a TLS certificate. Set a hostname containing only letters, digits, dots, and hyphens, then rerun."
detect_package_manager
detect_lan_ip
printf 'Detected: %s (%s); package manager: %s\n' "$OS_NAME" "$(uname -m)" "${PACKAGE_MANAGER:-none}"
select_database_mode

heading "Dependencies"
export PATH="${XDG_DATA_HOME:-$HOME/.local/share}/pms/toolchains/node/bin:${XDG_DATA_HOME:-$HOME/.local/share}/pms/toolchains/go/bin:$PATH"
if [[ "$SKIP_DEPS" -eq 1 ]]; then
    printf 'Dependency installation skipped; verifying required tools.\n'
fi
ensure_dependencies
if [[ "$DATABASE_MODE" == "docker" ]]; then
    ensure_docker_running
fi
if [[ "$OS_NAME" == "Linux" ]]; then
    ensure_backup_tools
fi
printf 'All required dependencies are available.\n'

heading "Configuration"
configure_env
printf 'Configuration is ready and existing values were preserved.\n'

heading "TLS"
ensure_tls

if [[ "$OS_NAME" == "Linux" ]]; then
    set_env_if_empty PMS_BACKUP_DIR "${XDG_DATA_HOME:-$HOME/.local/share}/pms/backups"
    [[ "$PMS_BACKUP_DIR" == /* ]] || fail "PMS_BACKUP_DIR must be absolute."
    backup_path="$(realpath -m "$PMS_BACKUP_DIR")"
    for protected_path in "$ROOT" "$PMS_ATTACHMENT_STORE" "$PMS_WEB_DIST"; do
        protected_path="$(realpath -m "$protected_path")"
        case "$backup_path/" in "$protected_path/"*) fail "PMS_BACKUP_DIR overlaps application data." ;; esac
        case "$protected_path/" in "$backup_path/"*) fail "PMS_BACKUP_DIR overlaps application data." ;; esac
    done
    mkdir -p "$PMS_BACKUP_DIR" "$HOME/.local/state/pms"
    chmod 700 "$PMS_BACKUP_DIR" "$HOME/.local/state/pms"
    exec 9>"$HOME/.local/state/pms/maintenance.lock"
    flock -w 3600 9
fi

heading "Database"
start_database
run_migrations
converge_database_passwords
printf 'Database is ready and migrations are current.\n'

heading "Seed administrator"
seed_administrator
printf 'Administrator account is ready; no demonstration data was loaded.\n'

heading "Build"
build_application
printf 'Production API and web bundle were built.\n'

heading "Start"
if [[ "$OS_NAME" == "Linux" ]]; then
    install_linux_services
elif [[ "$NO_START" -eq 1 ]]; then
    printf 'Server start skipped by --no-start.\n'
else
    start_application
fi
