#!/usr/bin/env bash
# Give this server a fixed address on the LAN and make the application reachable on it.
#
# Scans 192.168.1.254 downward to 192.168.1.2, claims the best free address, writes it
# into whichever network stack manages the active interface, reissues the TLS certificate
# so browsers accept the new address, opens the port, restarts the service, and proves the
# result with a real request.
set -euo pipefail

SUBNET_PREFIX="192.168.1"
SCAN_HIGH=254
SCAN_LOW=2
# Consumer routers hand out leases from a pool low in the range, so addresses at the top
# are the least likely to be handed to another machine later.
PREFERRED_LOW=200

DRY_RUN=0
ASSUME_YES=0
SKIP_DEPS=0
PRIVILEGED=0
PACKAGE_MANAGER=""
APT_UPDATED=0
HAVE_ARPING=0

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"
STATE_DIR="$ROOT/infra/network"
STATE_FILE="$STATE_DIR/static-ip.env"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
NETPLAN_FILE="/etc/netplan/99-pms-static.yaml"
NETWORKD_FILE="/etc/systemd/network/99-pms-static.network"

IFACE=""
CURRENT_IP=""
GATEWAY=""
CHOSEN_IP=""
STACK=""
STACK_LABEL=""
NM_CONNECTION=""
DNS_LIST=""
PORT=""
HOST_NAME=""
REUSED_EXISTING=0
BELOW_PREFERRED=0
CURRENT_PHASE="argument parsing"

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

unexpected_error() {
    printf 'ERROR: Failed during %s. Fix the error above, then rerun this command.\n' "$CURRENT_PHASE" >&2
    exit 1
}
trap unexpected_error ERR

note() {
    printf 'Note: %s\n' "$1"
}

step() {
    printf '\n== %s\n' "$1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

usage() {
    cat <<'USAGE'
Usage: bash scripts/set-static-ip.sh [--dry-run] [--yes] [--skip-deps]

Assigns this server a static IPv4 address on 192.168.1.0/24 and makes the application
reachable on it from other machines on the LAN.

  --dry-run     Detect, scan and report without changing anything.
  --yes         Skip the confirmation prompt. Required when stdin is not a terminal.
  --skip-deps   Do not install missing commands. Fail instead and let the operator
                install them.
  --help        Show this message.

Missing commands are installed automatically with apt, dnf, pacman, zypper or apk. The
network stack itself is never installed: replacing the stack that already manages the
interface would cut this machine off the network.

The script scans 192.168.1.254 downward to 192.168.1.2 and claims the highest free
address, preferring 192.168.1.254 to 192.168.1.200 so the address stays clear of the
router's DHCP pool. It then configures NetworkManager, netplan or systemd-networkd,
reissues the TLS certificate for the new address, opens the application port on ufw or
firewalld, restarts the pms-api user service, and verifies the result with an HTTPS
request. The outcome is written to infra/network/static-ip.env.

Reconfiguring the interface can drop an SSH session. Run it from a local console when
you can.
USAGE
}

# ---------------------------------------------------------------------------
# Privilege helpers
# ---------------------------------------------------------------------------

run_privileged() {
    if [[ "$(id -u)" -eq 0 ]]; then
        if ! "$@"; then
            fail "The privileged command failed. Run '$*' manually, resolve its error, then rerun this command."
        fi
        return
    fi
    if ! sudo -n "$@"; then
        fail "The privileged command failed. Run 'sudo $*' manually, resolve its error, then rerun this command."
    fi
}

# Runs a privileged command whose non-zero exit is information rather than an error,
# so it must not trip `fail` or the ERR trap.
privileged_probe() {
    if [[ "$(id -u)" -eq 0 ]]; then
        "$@" 2>/dev/null
        return $?
    fi
    sudo -n "$@" 2>/dev/null
    return $?
}

detect_privilege() {
    if [[ "$(id -u)" -eq 0 ]]; then
        PRIVILEGED=1
        return
    fi
    if command_exists sudo && sudo -n true >/dev/null 2>&1; then
        PRIVILEGED=1
        return
    fi
    PRIVILEGED=0
}

# ---------------------------------------------------------------------------
# Dependencies
#
# Every command this script runs is checked before it is used, and installed when it is
# missing, so a bare server does not fail partway through with a static address applied
# and the application unreachable.
# ---------------------------------------------------------------------------

# A privileged command whose failure is handled by the caller rather than fatal.
run_privileged_soft() {
    if [[ "$(id -u)" -eq 0 ]]; then
        "$@"
        return $?
    fi
    sudo -n "$@"
    return $?
}

detect_package_manager() {
    local candidate
    for candidate in apt-get dnf pacman zypper apk; do
        if command_exists "$candidate"; then
            PACKAGE_MANAGER="$candidate"
            return
        fi
    done
    PACKAGE_MANAGER=""
}

# Distributions disagree on package names for the same command.
package_for() {
    local tool=$1
    case "$tool" in
        ip)
            case "$PACKAGE_MANAGER" in
                dnf) printf 'iproute' ;;
                *) printf 'iproute2' ;;
            esac
            ;;
        awk) printf 'gawk' ;;
        openssl) printf 'openssl' ;;
        curl) printf 'curl' ;;
        grep) printf 'grep' ;;
        sed) printf 'sed' ;;
        tee|mktemp|tr|head|cut|cp|mv|rm|mkdir|chmod|cat|date|sleep|id|uname) printf 'coreutils' ;;
        arping)
            case "$PACKAGE_MANAGER" in
                apt-get) printf 'iputils-arping' ;;
                *) printf 'iputils' ;;
            esac
            ;;
        ping)
            case "$PACKAGE_MANAGER" in
                apt-get) printf 'iputils-ping' ;;
                *) printf 'iputils' ;;
            esac
            ;;
        *) printf '%s' "$tool" ;;
    esac
}

# Returns non-zero instead of exiting: the caller decides whether a failed install is
# fatal for the command it was installing.
install_packages() {
    case "$PACKAGE_MANAGER" in
        apt-get)
            if (( APT_UPDATED == 0 )); then
                run_privileged_soft apt-get update >/dev/null 2>&1 || true
                APT_UPDATED=1
            fi
            run_privileged_soft env DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" >/dev/null 2>&1
            ;;
        dnf)
            run_privileged_soft dnf --refresh install -y "$@" >/dev/null 2>&1
            ;;
        pacman)
            run_privileged_soft pacman -S --needed --noconfirm "$@" >/dev/null 2>&1
            ;;
        zypper)
            run_privileged_soft zypper --non-interactive refresh >/dev/null 2>&1 || true
            run_privileged_soft zypper --non-interactive install "$@" >/dev/null 2>&1
            ;;
        apk)
            run_privileged_soft apk --update-cache add --upgrade "$@" >/dev/null 2>&1
            ;;
        *)
            return 1
            ;;
    esac
}

# ensure_tool required <command>   installs it, or stops with an explanation.
# ensure_tool optional <command>   installs it if it can, and returns non-zero if not.
ensure_tool() {
    local requirement=$1 tool=$2 package reason=""
    command_exists "$tool" && return 0

    if (( DRY_RUN == 1 )); then
        reason="--dry-run changes nothing, so no package was installed"
    elif (( SKIP_DEPS == 1 )); then
        reason="--skip-deps is set"
    elif [[ -z "$PACKAGE_MANAGER" ]]; then
        reason="no supported package manager was found (apt, dnf, pacman, zypper or apk)"
    elif (( PRIVILEGED == 0 )); then
        reason="installing packages requires root or non-interactive sudo"
    fi

    if [[ -z "$reason" ]]; then
        package="$(package_for "$tool")"
        printf 'Installing the missing command %s from package %s.\n' "$tool" "$package"
        if install_packages "$package" && command_exists "$tool"; then
            return 0
        fi
        reason="installing package $package with $PACKAGE_MANAGER did not provide it"
    fi

    if [[ "$requirement" == "optional" ]]; then
        return 1
    fi
    fail "This script needs the command $tool, which is not installed, and $reason. Install $tool with this distribution's package manager, then rerun this command."
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

require_linux() {
    local os
    os="$(uname -s 2>/dev/null || true)"
    if [[ "$os" != "Linux" ]]; then
        fail "This script configures the Linux network stacks NetworkManager, netplan and systemd-networkd, and cannot run on $os. Assign the static address using this platform's own network settings, then set PMS_LISTEN_ADDR, reissue the TLS certificate for that address, and restart the service."
    fi
}

require_tools() {
    local tool
    for tool in ip awk openssl curl grep sed tr head tee mktemp cp mv rm mkdir chmod cat date sleep; do
        ensure_tool required "$tool"
    done

    # Duplicate address detection is the reliable probe, so it is worth installing. The
    # ping and ARP fallback still works where it cannot be had.
    if ensure_tool optional arping; then
        HAVE_ARPING=1
    else
        HAVE_ARPING=0
        ensure_tool required ping
        note "arping is unavailable, so free addresses are probed with ping and the ARP table. A host that ignores ICMP but answers ARP is still detected; one that does neither could be missed."
    fi
}

resolve_hostname() {
    local candidate=""
    for candidate in "$(hostname 2>/dev/null || true)" "$(uname -n 2>/dev/null || true)"; do
        if [[ "$candidate" =~ ^[A-Za-z0-9.-]+$ ]]; then
            HOST_NAME="$candidate"
            return
        fi
    done
    if [[ -r /etc/hostname ]]; then
        candidate="$(tr -d '[:space:]' </etc/hostname)"
        if [[ "$candidate" =~ ^[A-Za-z0-9.-]+$ ]]; then
            HOST_NAME="$candidate"
            return
        fi
    fi
    fail "The machine hostname cannot be placed in a TLS certificate. Set a hostname containing only letters, digits, dots, and hyphens, then rerun this command."
}

load_env() {
    [[ -f "$ENV_FILE" ]] || fail "$ENV_FILE does not exist. Run 'bash scripts/setup.sh' on this machine first, then rerun this command."
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
    PORT="${PMS_LISTEN_ADDR:-:8443}"
    PORT="${PORT##*:}"
    PORT="${PORT%]}"
    [[ "$PORT" =~ ^[0-9]+$ ]] && (( PORT >= 1 && PORT <= 65535 )) || fail "PMS_LISTEN_ADDR must end in a valid TCP port. Fix it in $ENV_FILE, then rerun this command."
    [[ -n "${PMS_TLS_CERT_PATH:-}" ]] || fail "PMS_TLS_CERT_PATH is not set in $ENV_FILE. Set it to the server certificate path, then rerun this command."
    [[ -n "${PMS_TLS_KEY_PATH:-}" ]] || fail "PMS_TLS_KEY_PATH is not set in $ENV_FILE. Set it to the private key path, then rerun this command."
}

# ---------------------------------------------------------------------------
# Network detection
# ---------------------------------------------------------------------------

detect_network() {
    local route
    route="$(ip -4 route get 1.1.1.1 2>/dev/null || true)"
    IFACE="$(printf '%s' "$route" | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')"
    CURRENT_IP="$(printf '%s' "$route" | awk '{for (i=1; i<=NF; i++) if ($i == "src") {print $(i+1); exit}}')"
    [[ -n "$IFACE" ]] || fail "Could not determine the outbound network interface. Connect this machine to the LAN, then rerun this command."

    GATEWAY="$(ip -4 route show default 2>/dev/null | awk -v dev="$IFACE" '$0 ~ ("dev " dev) {for (i=1; i<=NF; i++) if ($i == "via") {print $(i+1); exit}}')"
    if [[ -z "$GATEWAY" ]]; then
        GATEWAY="$(ip -4 route show default 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "via") {print $(i+1); exit}}')"
    fi
    [[ "$GATEWAY" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail "Could not determine the default gateway on $IFACE. Connect this machine to the router, then rerun this command."

    # Refuse to move the machine onto a subnet its router does not serve: the address
    # would be unreachable and this session would end with it.
    if [[ "$GATEWAY" != "$SUBNET_PREFIX."* ]]; then
        fail "The default gateway is $GATEWAY, which is outside $SUBNET_PREFIX.0/24. This script only assigns addresses on $SUBNET_PREFIX.0/24, and an address off the router's own subnet would be unreachable. Connect this machine to the $SUBNET_PREFIX.0/24 network, then rerun this command."
    fi
}

detect_stack() {
    if command_exists nmcli; then
        NM_CONNECTION="$(nmcli -t -g NAME,DEVICE connection show --active 2>/dev/null | awk -F: -v dev="$IFACE" '$2 == dev {print $1; exit}')"
        if [[ -n "$NM_CONNECTION" ]]; then
            STACK="networkmanager"
            STACK_LABEL="NetworkManager (connection \"$NM_CONNECTION\")"
            return
        fi
    fi
    if command_exists netplan && [[ -d /etc/netplan ]]; then
        STACK="netplan"
        STACK_LABEL="netplan ($NETPLAN_FILE)"
        return
    fi
    if command_exists systemctl && systemctl is-active --quiet systemd-networkd 2>/dev/null; then
        STACK="networkd"
        STACK_LABEL="systemd-networkd ($NETWORKD_FILE)"
        return
    fi
    fail "No supported network stack manages $IFACE. This script configures NetworkManager, netplan or systemd-networkd. Assign the static address with this system's own network tooling, then rerun with the address already in place."
}

detect_dns() {
    local servers=""
    if [[ "$STACK" == "networkmanager" ]]; then
        servers="$(nmcli -t -g IP4.DNS device show "$IFACE" 2>/dev/null | tr '\n' ' ' || true)"
    fi
    if [[ -z "${servers// /}" ]] && command_exists resolvectl; then
        servers="$(resolvectl dns "$IFACE" 2>/dev/null | sed 's/^.*)://' | tr '\n' ' ' || true)"
    fi
    if [[ -z "${servers// /}" ]] && [[ -r /etc/resolv.conf ]]; then
        servers="$(awk '/^nameserver/ {print $2}' /etc/resolv.conf | tr '\n' ' ')"
    fi
    # 127.0.0.53 is the systemd-resolved stub. Writing it back as the upstream resolver
    # would point the stub at itself and break name resolution.
    local cleaned="" server
    for server in $servers; do
        [[ "$server" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || continue
        [[ "$server" != 127.* ]] || continue
        cleaned="${cleaned:+$cleaned }$server"
    done
    if [[ -z "$cleaned" ]]; then
        # The router answers DNS on nearly every consumer LAN, and losing name
        # resolution on a server that had it is a regression.
        cleaned="$GATEWAY"
        note "No upstream DNS server was detected, so the gateway $GATEWAY will be used as the resolver."
    fi
    DNS_LIST="$cleaned"
}

# ---------------------------------------------------------------------------
# Address selection
# ---------------------------------------------------------------------------

existing_static_address() {
    local address=""
    case "$STACK" in
        networkmanager)
            if [[ "$(nmcli -g ipv4.method connection show "$NM_CONNECTION" 2>/dev/null || true)" == "manual" ]]; then
                address="$(nmcli -g ipv4.addresses connection show "$NM_CONNECTION" 2>/dev/null | tr ',' '\n' | sed 's#/.*##' | awk 'NF {print; exit}')"
            fi
            ;;
        netplan)
            if [[ -r "$NETPLAN_FILE" ]]; then
                address="$(awk '/addresses:/ {gsub(/[][,]/, " "); for (i=1; i<=NF; i++) if ($i ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\//) {split($i, a, "/"); print a[1]; exit}}' "$NETPLAN_FILE" || true)"
            fi
            ;;
        networkd)
            if [[ -r "$NETWORKD_FILE" ]]; then
                address="$(awk -F= '/^Address=/ {split($2, a, "/"); gsub(/ /, "", a[1]); print a[1]; exit}' "$NETWORKD_FILE" || true)"
            fi
            ;;
    esac
    [[ "$address" == "$SUBNET_PREFIX."* ]] || return 1
    printf '%s' "$address"
}

# An address is free only when nothing answers it. arping's duplicate address detection
# is the right probe; ICMP plus the ARP table is the fallback where arping is absent.
ip_is_free() {
    local candidate=$1 neigh
    if (( HAVE_ARPING == 1 && PRIVILEGED == 1 )); then
        if privileged_probe arping -D -I "$IFACE" -c 2 -w 3 "$candidate" >/dev/null; then
            return 0
        fi
        return 1
    fi
    if ping -c 2 -W 1 "$candidate" >/dev/null 2>&1; then
        return 1
    fi
    neigh="$(ip neigh show "$candidate" dev "$IFACE" 2>/dev/null || true)"
    if [[ -n "$neigh" && "$neigh" != *FAILED* && "$neigh" != *INCOMPLETE* ]]; then
        return 1
    fi
    return 0
}

choose_address() {
    local existing octet candidate
    if existing="$(existing_static_address)"; then
        CHOSEN_IP="$existing"
        REUSED_EXISTING=1
        printf 'This interface already carries the static address %s. Reusing it.\n' "$CHOSEN_IP"
        return
    fi

    printf 'Scanning %s.%s down to %s.%s for a free address. This takes a moment.\n' \
        "$SUBNET_PREFIX" "$SCAN_HIGH" "$SUBNET_PREFIX" "$SCAN_LOW"
    for (( octet = SCAN_HIGH; octet >= SCAN_LOW; octet-- )); do
        candidate="$SUBNET_PREFIX.$octet"
        [[ "$candidate" != "$GATEWAY" ]] || continue
        # Claiming this host's own DHCP lease invites a conflict at renewal time.
        [[ "$candidate" != "$CURRENT_IP" ]] || continue
        if (( octet < PREFERRED_LOW && BELOW_PREFERRED == 0 )); then
            BELOW_PREFERRED=1
            note "Every address from $SUBNET_PREFIX.$SCAN_HIGH down to $SUBNET_PREFIX.$PREFERRED_LOW is taken, so the search continues into the range routers usually use for DHCP leases."
        fi
        if ip_is_free "$candidate"; then
            CHOSEN_IP="$candidate"
            break
        fi
    done
    [[ -n "$CHOSEN_IP" ]] || fail "No address between $SUBNET_PREFIX.$SCAN_HIGH and $SUBNET_PREFIX.$SCAN_LOW is free. Free an address on the LAN, then rerun this command."
    printf 'Selected %s.\n' "$CHOSEN_IP"
    if (( BELOW_PREFERRED == 1 )); then
        note "Exclude $CHOSEN_IP from the router's DHCP pool, or reserve it for this machine, so the router does not lease it to another device."
    fi
}

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------

confirm() {
    local reply
    printf '\nAbout to apply this configuration:\n'
    printf '  Interface:       %s\n' "$IFACE"
    printf '  Current address: %s\n' "${CURRENT_IP:-none}"
    printf '  Static address:  %s/24\n' "$CHOSEN_IP"
    printf '  Gateway:         %s\n' "$GATEWAY"
    printf '  DNS:             %s\n' "$DNS_LIST"
    printf '  Network stack:   %s\n' "$STACK_LABEL"
    printf '  Application URL: https://%s:%s\n' "$CHOSEN_IP" "$PORT"
    printf '\nReconfiguring the interface can drop an SSH session.\n'

    if (( ASSUME_YES == 1 )); then
        printf 'Proceeding because --yes was given.\n'
        return
    fi
    if [[ ! -t 0 ]]; then
        fail "This command needs confirmation but its input is not a terminal. Rerun it with --yes once you are ready for the interface to be reconfigured."
    fi
    printf 'Type yes to continue: '
    read -r reply
    [[ "$reply" == "yes" ]] || fail "Cancelled. Nothing was changed."
}

# ---------------------------------------------------------------------------
# Applying the address
# ---------------------------------------------------------------------------

write_privileged_file() {
    local path=$1 content=$2
    printf '%s' "$content" | run_privileged tee "$path" >/dev/null
    run_privileged chmod 600 "$path"
}

apply_networkmanager() {
    local dns="${DNS_LIST// /,}"
    run_privileged nmcli connection modify "$NM_CONNECTION" \
        ipv4.addresses "$CHOSEN_IP/24" \
        ipv4.gateway "$GATEWAY" \
        ipv4.dns "$dns" \
        ipv4.method manual
    run_privileged nmcli connection up "$NM_CONNECTION"
}

apply_netplan() {
    local dns_yaml="" server other
    for server in $DNS_LIST; do
        dns_yaml="${dns_yaml:+$dns_yaml, }$server"
    done
    for other in /etc/netplan/*.yaml /etc/netplan/*.yml; do
        [[ -e "$other" ]] || continue
        [[ "$other" != "$NETPLAN_FILE" ]] || continue
        if grep -q "$IFACE" "$other" 2>/dev/null; then
            note "$other also configures $IFACE. netplan merges its files in name order, so $NETPLAN_FILE takes precedence. Remove the $IFACE section from $other if the address does not take effect."
        fi
    done
    write_privileged_file "$NETPLAN_FILE" "\
# Written by scripts/set-static-ip.sh. Delete this file to return $IFACE to DHCP.
network:
  version: 2
  ethernets:
    $IFACE:
      dhcp4: false
      addresses: [$CHOSEN_IP/24]
      routes:
        - to: default
          via: $GATEWAY
      nameservers:
        addresses: [$dns_yaml]
"
    run_privileged netplan apply
}

apply_networkd() {
    local dns_lines="" server
    for server in $DNS_LIST; do
        dns_lines="${dns_lines}DNS=$server"$'\n'
    done
    write_privileged_file "$NETWORKD_FILE" "\
# Written by scripts/set-static-ip.sh. Delete this file to return $IFACE to DHCP.
[Match]
Name=$IFACE

[Network]
Address=$CHOSEN_IP/24
Gateway=$GATEWAY
$dns_lines"
    if ! privileged_probe networkctl reload >/dev/null; then
        run_privileged systemctl restart systemd-networkd
    fi
}

wait_for_address() {
    local deadline=$((SECONDS + 20))
    while (( SECONDS < deadline )); do
        if ip -4 addr show dev "$IFACE" 2>/dev/null | grep -q "inet $CHOSEN_IP/"; then
            printf 'The interface %s now carries %s.\n' "$IFACE" "$CHOSEN_IP"
            return
        fi
        sleep 2
    done
    fail "$IFACE did not come up with $CHOSEN_IP within 20 seconds. Run 'ip -4 addr show dev $IFACE' to see its current state, resolve the error reported by the network stack, then rerun this command."
}

apply_address() {
    case "$STACK" in
        networkmanager) apply_networkmanager ;;
        netplan) apply_netplan ;;
        networkd) apply_networkd ;;
    esac
    wait_for_address
}

# ---------------------------------------------------------------------------
# Making the application reachable
# ---------------------------------------------------------------------------

update_listen_addr() {
    local desired="0.0.0.0:$PORT" tmp
    case "${PMS_LISTEN_ADDR:-}" in
        :*|0.0.0.0:*|\[::\]:*)
            printf 'PMS_LISTEN_ADDR is %s, which already accepts connections from the LAN.\n' "$PMS_LISTEN_ADDR"
            return
            ;;
    esac
    cp -p "$ENV_FILE" "$ENV_FILE.bak-$STAMP"
    chmod 600 "$ENV_FILE.bak-$STAMP"
    tmp="$(mktemp)"
    if grep -q '^PMS_LISTEN_ADDR=' "$ENV_FILE"; then
        awk -v repl="PMS_LISTEN_ADDR='$desired'" '
            !replaced && /^PMS_LISTEN_ADDR=/ { print repl; replaced = 1; next }
            { print }
        ' "$ENV_FILE" >"$tmp"
    else
        cat "$ENV_FILE" >"$tmp"
        printf "PMS_LISTEN_ADDR='%s'\n" "$desired" >>"$tmp"
    fi
    # Overwrite in place so the file keeps its ownership and inode.
    cat "$tmp" >"$ENV_FILE"
    rm -f "$tmp"
    chmod 600 "$ENV_FILE"
    PMS_LISTEN_ADDR="$desired"
    printf 'Set PMS_LISTEN_ADDR to %s. The previous file is at %s.\n' "$desired" "$ENV_FILE.bak-$STAMP"
}

ensure_certificate() {
    local cert="$PMS_TLS_CERT_PATH" key="$PMS_TLS_KEY_PATH"
    if [[ -f "$cert" ]] && openssl x509 -noout -ext subjectAltName -in "$cert" 2>/dev/null | grep -Eq "IP Address:${CHOSEN_IP}([^0-9]|$)"; then
        printf 'The existing TLS certificate already covers %s.\n' "$CHOSEN_IP"
        return
    fi
    [[ ! -e "$cert" ]] || mv "$cert" "$cert.bak-$STAMP"
    [[ ! -e "$key" ]] || mv "$key" "$key.bak-$STAMP"
    mkdir -p "$(dirname "$cert")" "$(dirname "$key")"
    if ! openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 \
        -keyout "$key" \
        -out "$cert" \
        -subj "/CN=localhost" \
        -addext "subjectAltName=DNS:localhost,DNS:$HOST_NAME,IP:127.0.0.1,IP:$CHOSEN_IP" >/dev/null 2>&1; then
        fail "OpenSSL could not create the TLS certificate. Run 'openssl version', repair OpenSSL, then rerun this command."
    fi
    chmod 600 "$key"
    printf 'Issued a TLS certificate for localhost, %s, 127.0.0.1 and %s.\n' "$HOST_NAME" "$CHOSEN_IP"
    if [[ -e "$cert.bak-$STAMP" ]]; then
        note "The previous certificate is at $cert.bak-$STAMP. Every browser on the LAN must accept the new one."
    fi
}

open_firewall() {
    if command_exists ufw && privileged_probe ufw status | head -1 | grep -q 'Status: active'; then
        run_privileged ufw allow "$PORT/tcp"
        printf 'Opened %s/tcp on ufw.\n' "$PORT"
        return
    fi
    if command_exists firewall-cmd && systemctl is-active --quiet firewalld 2>/dev/null; then
        run_privileged firewall-cmd --permanent --add-port="$PORT/tcp"
        run_privileged firewall-cmd --reload
        printf 'Opened %s/tcp on firewalld.\n' "$PORT"
        return
    fi
    note "Neither ufw nor firewalld is active, so no firewall rule was needed for port $PORT."
}

# pms-api.service is a systemd user unit, so it must be restarted as the application
# account rather than as root, even when this script runs under sudo.
as_app_user() {
    local user=$1 uid=$2
    shift 2
    if [[ "$(id -un)" == "$user" ]]; then
        XDG_RUNTIME_DIR="/run/user/$uid" "$@"
        return $?
    fi
    if command_exists runuser; then
        privileged_probe runuser -u "$user" -- env "XDG_RUNTIME_DIR=/run/user/$uid" "$@"
        return $?
    fi
    privileged_probe sudo -u "$user" env "XDG_RUNTIME_DIR=/run/user/$uid" "$@"
}

restart_service() {
    local user uid
    user="${SUDO_USER:-$(id -un)}"
    uid="$(id -u "$user" 2>/dev/null || true)"
    if [[ -z "$uid" ]]; then
        note "Could not resolve the application account, so pms-api.service was not restarted. Restart it yourself with 'systemctl --user restart pms-api.service'."
        return
    fi
    if ! as_app_user "$user" "$uid" systemctl --user cat pms-api.service >/dev/null 2>&1; then
        note "pms-api.service is not installed for $user, so nothing was restarted. Start the application yourself, or run 'bash scripts/setup.sh' to install the service."
        return
    fi
    if as_app_user "$user" "$uid" systemctl --user restart pms-api.service >/dev/null 2>&1; then
        printf 'Restarted pms-api.service as %s.\n' "$user"
        return
    fi
    note "pms-api.service did not restart cleanly. Run 'systemctl --user status pms-api.service' as $user to see why."
}

verify_reachable() {
    local deadline=$((SECONDS + 30)) code=""
    while (( SECONDS < deadline )); do
        # -k is correct here: the certificate is self-signed by design.
        code="$(curl -k -sS -o /dev/null -w '%{http_code}' --max-time 5 "https://$CHOSEN_IP:$PORT/" 2>/dev/null || true)"
        if [[ "$code" =~ ^[0-9]{3}$ ]] && [[ "$code" != "000" ]]; then
            printf 'The application answered on https://%s:%s with HTTP %s.\n' "$CHOSEN_IP" "$PORT" "$code"
            return 0
        fi
        sleep 2
    done
    return 1
}

write_state() {
    mkdir -p "$STATE_DIR"
    cat >"$STATE_FILE" <<STATE
# Written by scripts/set-static-ip.sh. Generated; not checked into git.
PMS_STATIC_IP='$CHOSEN_IP'
PMS_STATIC_IFACE='$IFACE'
PMS_STATIC_GATEWAY='$GATEWAY'
PMS_STATIC_STACK='$STACK'
PMS_STATIC_PORT='$PORT'
PMS_STATIC_APPLIED_AT='$(date -u +%Y-%m-%dT%H:%M:%SZ)'
STATE
    chmod 600 "$STATE_FILE"
    printf 'Recorded the result in %s.\n' "$STATE_FILE"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

while (( $# > 0 )); do
    case "$1" in
        --dry-run) DRY_RUN=1 ;;
        --yes) ASSUME_YES=1 ;;
        --skip-deps) SKIP_DEPS=1 ;;
        --help|-h) usage; exit 0 ;;
        *) usage; fail "Unknown option: $1" ;;
    esac
    shift
done

CURRENT_PHASE="preflight"
require_linux
detect_privilege
detect_package_manager
require_tools
resolve_hostname
load_env
if (( DRY_RUN == 0 && PRIVILEGED == 0 )); then
    fail "Changing the network configuration requires root or non-interactive sudo. Run 'sudo -v' and rerun this command, or rerun it with --dry-run to see what it would do."
fi

CURRENT_PHASE="network detection"
step "Detecting the network"
detect_network
detect_stack
detect_dns
printf 'Interface %s, current address %s, gateway %s, managed by %s.\n' \
    "$IFACE" "${CURRENT_IP:-none}" "$GATEWAY" "$STACK_LABEL"

CURRENT_PHASE="address selection"
step "Choosing an address"
if (( DRY_RUN == 1 && PRIVILEGED == 0 )); then
    note "Running without root, so free addresses are probed with ping and the ARP table instead of arping."
fi
choose_address

if (( DRY_RUN == 1 )); then
    step "Dry run"
    printf 'Would assign %s/24 to %s via %s.\n' "$CHOSEN_IP" "$IFACE" "$STACK_LABEL"
    printf 'Would set PMS_LISTEN_ADDR to 0.0.0.0:%s in %s.\n' "$PORT" "$ENV_FILE"
    printf 'Would reissue %s so it covers %s.\n' "$PMS_TLS_CERT_PATH" "$CHOSEN_IP"
    printf 'Would open %s/tcp on the active firewall and restart pms-api.service.\n' "$PORT"
    printf 'Would record the result in %s.\n' "$STATE_FILE"
    printf '\nStatic IP: %s\n' "$CHOSEN_IP"
    printf 'App URL: https://%s:%s\n' "$CHOSEN_IP" "$PORT"
    printf 'Reachable: not tested — this was a dry run.\n'
    exit 0
fi

confirm

CURRENT_PHASE="applying the static address"
if (( REUSED_EXISTING == 1 )); then
    step "Keeping the existing static address"
else
    step "Applying the static address"
    apply_address
fi

CURRENT_PHASE="making the application reachable"
step "Making the application reachable"
update_listen_addr
ensure_certificate
open_firewall
restart_service

CURRENT_PHASE="verification"
step "Verifying"
REACHABLE=0
verify_reachable && REACHABLE=1

write_state

printf '\nStatic IP: %s\n' "$CHOSEN_IP"
printf 'App URL: https://%s:%s\n' "$CHOSEN_IP" "$PORT"
if (( REACHABLE == 1 )); then
    printf 'Reachable: yes\n'
else
    printf 'Reachable: no — nothing answered https://%s:%s within 30 seconds. Check '"'"'systemctl --user status pms-api.service'"'"', confirm PMS_LISTEN_ADDR is 0.0.0.0:%s in %s, and confirm port %s is open.\n' \
        "$CHOSEN_IP" "$PORT" "$PORT" "$ENV_FILE" "$PORT"
    exit 1
fi
