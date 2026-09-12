#!/usr/bin/env bash
set -euo pipefail
umask 077
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"
set -a
. ./.env
set +a
: "${PMS_DATABASE_OWNER_URL:?Missing database owner URL}"
: "${PMS_ATTACHMENT_STORE:?Missing attachment store}"
: "${PMS_BACKUP_DIR:?Run setup to configure the backup directory}"
unset PMS_KEK PMS_DATABASE_APP_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD

# Setup and backups must never mutate the database or stop the app concurrently.
mkdir -p "$HOME/.local/state/pms"
exec 9>"$HOME/.local/state/pms/maintenance.lock"
flock -w 3600 9
[[ -d "$PMS_BACKUP_DIR" && -w "$PMS_BACKUP_DIR" ]] || { echo 'Backup directory is unavailable.' >&2; exit 1; }
backup_dir="$(realpath "$PMS_BACKUP_DIR")"
store_dir="$(realpath "$PMS_ATTACHMENT_STORE")"
for protected_dir in "$ROOT" "$store_dir" "$(realpath "${PMS_WEB_DIST:-$ROOT/web/dist}")"; do
    case "$backup_dir/" in "$protected_dir/"*) echo 'Backup directory overlaps application data.' >&2; exit 1 ;; esac
    case "$protected_dir/" in "$backup_dir/"*) echo 'Backup directory overlaps application data.' >&2; exit 1 ;; esac
done
backup_date="$(TZ=Africa/Cairo date +%F)"
final_dir="$backup_dir/$backup_date"
if [[ -e "$final_dir" ]]; then
    (cd "$final_dir" && sha256sum --check --status SHA256SUMS)
    echo 'Today already has a verified backup.'
    exit 0
fi
staging="$(mktemp -d "$backup_dir/.incomplete.XXXXXX")"
restart_required=0
cleanup() {
    local status=$?
    trap - EXIT
    if [[ "$restart_required" -eq 1 ]]; then
        systemctl --user start pms-api.service || status=1
    fi
    rm -rf -- "$staging"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
systemctl --user is-active --quiet pms-api.service || { echo 'The app must be running before a backup.' >&2; exit 1; }
restart_required=1
systemctl --user stop pms-api.service
# Credentials are passed via environment, never command-line arguments or archive files.
node "$ROOT/scripts/database-tool.cjs" pg_dump --no-password --format=custom --file="$staging/database.dump"
pg_restore --list "$staging/database.dump" >/dev/null
tar -cf "$staging/attachments.tar" -C "$store_dir" .
systemctl --user start pms-api.service
restart_required=0
(cd "$staging" && sha256sum database.dump attachments.tar >SHA256SUMS && sha256sum --check --status SHA256SUMS)
mv -- "$staging" "$final_dir"

# Keep three completed daily snapshots. Failed runs never prune good backups.
shopt -s nullglob
backups=()
for candidate in "$backup_dir"/????-??-??; do
    [[ -d "$candidate" && ! -L "$candidate" && -f "$candidate/SHA256SUMS" ]] || continue
    [[ "${candidate##*/}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || continue
    backups+=("$candidate")
done
for ((i=0; i<${#backups[@]}-3; i++)); do
    rm -rf -- "${backups[i]}"
done
printf 'Backup completed: %s\n' "$final_dir"
