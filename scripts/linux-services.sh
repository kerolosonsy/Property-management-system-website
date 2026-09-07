# Sourced by setup.sh; uses its resolved paths and package installation helpers.
ensure_backup_tools() {
    local candidate
    if [[ "$SKIP_DEPS" -eq 0 ]]; then
        case "$PACKAGE_MANAGER" in
            apt-get) install_postgresql_apt postgresql-client-17 ;;
            dnf|zypper) install_packages postgresql17 ;;
            pacman) ensure_pacman_offers_postgresql_17; install_packages postgresql ;;
            *) fail "Automatic startup and backups require a systemd Linux distribution." ;;
        esac
    fi
    for candidate in /usr/lib/postgresql/17/bin /usr/pgsql-17/bin /usr/lib/postgresql17/bin; do
        [[ ! -x "$candidate/pg_dump" ]] || export PATH="$candidate:$PATH"
    done
    for candidate in node pg_dump pg_restore flock realpath sha256sum tar; do
        command_exists "$candidate" || fail "$candidate is required for automatic backups. Install it and rerun setup."
    done
    [[ "$(pg_dump --version)" =~ \ 17\. ]] || fail "PostgreSQL 17 client tools are required for backups."
}

# systemd expands % specifiers in every directive; ExecStart also expands dollars.
systemd_quote() {
    local quoted=$1
    [[ "$quoted" != *$'\n'* && "$quoted" != *$'\r'* ]] || fail "Service paths cannot contain newlines."
    quoted=${quoted//\\/\\\\}
    quoted=${quoted//\"/\\\"}
    quoted=${quoted//%/%%}
    printf '"%s"' "$quoted"
}

install_linux_services() {
    local unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user" service_script backup_script
    if [[ "$DATABASE_MODE" == "docker" ]]; then
        run_privileged systemctl enable docker
    fi
    run_privileged loginctl enable-linger "$(id -un)"
    export XDG_RUNTIME_DIR="/run/user/$(id -u)"
    systemctl --user show-environment >/dev/null || fail "The systemd user manager is unavailable. Log in as the app user and rerun setup without sudo."
    mkdir -p "$unit_dir"
    service_script=${ROOT//\$/\$\$}/scripts/run-service.sh
    backup_script=${ROOT//\$/\$\$}/scripts/backup.sh
    cat >"$unit_dir/pms-api.service" <<UNIT
[Unit]
Description=Property Management System
StartLimitIntervalSec=0

[Service]
ExecStart=/bin/bash $(systemd_quote "$service_script")
Environment=$(systemd_quote "PATH=$PATH")
Restart=always
RestartSec=5
TimeoutStopSec=30
UMask=0077

[Install]
WantedBy=default.target
UNIT
    cat >"$unit_dir/pms-backup.service" <<UNIT
[Unit]
Description=Daily PMS database and encrypted attachment backup

[Service]
Type=oneshot
ExecStart=/bin/bash $(systemd_quote "$backup_script")
ExecStopPost=/usr/bin/systemctl --user start pms-api.service
Environment=$(systemd_quote "PATH=$PATH")
TimeoutStartSec=6h
UMask=0077
UNIT
    cat >"$unit_dir/pms-backup.timer" <<'UNIT'
[Unit]
Description=Back up PMS daily at 02:00 Cairo time

[Timer]
OnCalendar=*-*-* 02:00:00 Africa/Cairo
Persistent=true

[Install]
WantedBy=timers.target
UNIT
    systemctl --user daemon-reload
    systemctl --user enable pms-api.service pms-backup.timer
    if [[ "$NO_START" -eq 0 ]]; then
        stop_managed_server "${XDG_STATE_HOME:-$HOME/.local/state}/pms/pms-api.pid"
        systemctl --user restart pms-api.service
        systemctl --user start pms-backup.timer
        local attempt
        for ((attempt=0; attempt<60; attempt++)); do
            if health_ready "$(listen_port)"; then
                printf 'App enabled at boot: https://localhost:%s\n' "$(listen_port)"
                if [[ "$LISTEN_ALL_INTERFACES" -eq 1 ]]; then
                    printf 'LAN URL: https://%s:%s\n' "$LAN_IP" "$(listen_port)"
                fi
                printf 'Daily backups: %s (latest three successful days).\n' "$PMS_BACKUP_DIR"
                return
            fi
            sleep 1
        done
        fail "The service did not become healthy. Inspect: journalctl --user -u pms-api.service"
    fi
}
