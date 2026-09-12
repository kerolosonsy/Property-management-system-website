#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
set -a
# The shell expands the portable paths in .env; systemd EnvironmentFile does not.
. ./.env
set +a
unset PMS_DATABASE_OWNER_URL PMS_ADMIN_PASSWORD DEV_ADMIN_PASSWORD
exec ./api/bin/pms-api
