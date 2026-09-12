# Linux startup and daily backups

Run setup as the regular account that owns the checkout on a systemd Linux machine:

```bash
sudo -v
bash scripts/setup.sh
```

Setup enables `pms-api.service` and `pms-backup.timer` in that account's systemd user
manager, and enables lingering so the app starts at boot without a login. The app
restarts after exit, including when PostgreSQL is not ready yet. Keep the checkout
and its home directory available at boot. Re-run setup after moving the checkout.
Use a single checkout per account: the service names are fixed.

The timer runs at **02:00 Africa/Cairo**. A missed run is triggered when the timer
next starts. Each successful day creates one directory named `YYYY-MM-DD`. The
latest **three successful daily snapshots** are retained; a failed backup does not
remove older ones. A second run on the same day verifies and reuses that snapshot.
`--no-start` installs/enables the units for the next boot without starting them now;
it does not stop an already running service or timer.

Each snapshot contains:

- `database.dump`: PostgreSQL 17 custom-format database dump, including schema,
  records, audit history, ownership and grants. Cluster roles/passwords are not included.
- `attachments.tar`: the encrypted attachment store, unchanged.
- `SHA256SUMS`: checksums of both files.

The backup briefly stops the app while copying both stores, so uploads and removals
cannot make them disagree. It restarts the app on success or failure. Systemd also
requests an app restart if the backup process is terminated. Duration depends on
how much data there is. Do not run separate API processes or offline database edits
during backups. Setup and backups share a maintenance lock.

## Location and key recovery

The default is `${XDG_DATA_HOME:-$HOME/.local/share}/pms/backups`. To use a separate
local disk, set an absolute, quoted `PMS_BACKUP_DIR` in `.env`, then rerun setup.
The directory must be writable by the app account and available for each backup.
For a removable disk, arrange its mount at boot before enabling the timer; setup
creates the configured directory, so verify the disk is mounted first.

A backup on the same disk helps with accidental edits but will be lost with that
disk. Prefer a separate encrypted local disk. Database metadata is not additionally
encrypted by this script; sensitive values and attachment contents retain their
existing encryption. Backup directories are private to the account.

Keep the exact `PMS_KEK` in a separate protected offline copy, such as an encrypted
USB device. These snapshots intentionally exclude `.env`, TLS keys and the KEK.
Without the original KEK, the attachments and sensitive fields cannot be recovered.
Preserve the application revision and configuration needed to rebuild it as well.
Nothing is uploaded to an external backup service.

## Check and operate

Run these as the same account used for setup, without `sudo`:

```bash
systemctl --user status pms-api.service
systemctl --user list-timers pms-backup.timer
systemctl --user start pms-backup.service
journalctl --user -u pms-backup.service -n 50 --no-pager
journalctl --user -u pms-api.service -n 50 --no-pager
```

Use `systemctl --user restart pms-api.service` after an environment change.
To keep the app stopped for maintenance, stop the timer and any running backup
first, then stop the app (the backup's exit hook otherwise starts it again):

```bash
systemctl --user stop pms-backup.timer pms-backup.service
systemctl --user stop pms-api.service
```

A failed dump, unavailable backup directory, checksum error or full disk makes the
backup service fail and retains existing snapshots. Inspect the journal, correct
the cause, and run `systemctl --user start pms-backup.service` again. The journal is
the failure indicator; email or other notifications are not configured.

## Restore a snapshot

First rehearse on a disposable installation. The following procedure **replaces
its current database and attachment store**. Use a trusted snapshot and its original
KEK. On a fresh machine, run setup with `--no-start` to create PostgreSQL 17, the
`pms_owner` and `pms_app` roles, configuration and application build. Restore the
original KEK in `.env` before opening existing encrypted data. Keep the new machine's
database credentials and paths. Do not run setup again until the restore is checked.

Run from the repository root in Bash, as the setup account. Set `BACKUP` to the
absolute path of the snapshot to restore:

```bash
BACKUP='/absolute/path/to/backups/YYYY-MM-DD'
systemctl --user stop pms-backup.timer pms-backup.service
systemctl --user stop pms-api.service
set -a
. ./.env
set +a
export PATH="${XDG_DATA_HOME:-$HOME/.local/share}/pms/toolchains/node/bin:/usr/lib/postgresql/17/bin:/usr/pgsql-17/bin:/usr/lib/postgresql17/bin:$PATH"
(cd "$BACKUP" && sha256sum --check SHA256SUMS)
```

Stop if verification fails. Then restore the database (existing `pms` objects will
be dropped and recreated) and extract the matching attachments into a new directory:

```bash
node scripts/database-tool.cjs pg_restore --no-password --exit-on-error \
  --clean --if-exists "$BACKUP/database.dump"
RESTORE_STORE="${PMS_ATTACHMENT_STORE}.restore-$(date +%s)"
mkdir -m 700 "$RESTORE_STORE"
tar -xf "$BACKUP/attachments.tar" -C "$RESTORE_STORE"
```

Stop on any error. Change only `PMS_ATTACHMENT_STORE` in `.env` to the absolute
`RESTORE_STORE` path. Keep the previous store until the restored data is verified.
Start the app and check health, sign in with the backed-up account credentials,
open an old attachment and a sensitive field, and inspect the audit history:

```bash
systemctl --user start pms-api.service
curl -k --fail https://localhost:8443/api/v1/health
```

Use the configured port if it differs from 8443. Once the restore is verified,
resume scheduling with `systemctl --user start pms-backup.timer`.

## Verification procedure and status

On a Linux test machine:

1. Run setup, reboot, and verify the health URL from another machine before logging
   in to the server. Check that the timer has a next run time in Cairo.
2. Create a property and upload an attachment. Trigger `pms-backup.service` and
   verify the app resumes, both archive checksums pass, and the dump is readable
   with `pg_restore --list`.
3. Repeat on the same day: the existing snapshot must be reused. Across four
   successful dates, only the latest three must remain.
4. Make the backup destination unavailable, trigger the service and check that it
   fails, existing snapshots remain, and the app is running. Restore the destination.
5. Rehearse the restore above on a disposable installation; verify an attachment,
   sensitive field and audit history against the original.

Verified on 2026-09-07 in a disposable PostgreSQL 17 Linux container, with service
commands simulated: real dump/restore and restored grants; byte-identical restored
attachments; three-snapshot retention; same-day reuse; cleanup and app restart
requests after dump failure and termination; service unit generation and path quoting.
Additional host checks passed for credential redaction, portable service paths,
removal of administrator credentials from the app environment, Bash/Node syntax,
and whitespace checks. No existing application data was used.

Actual reboot, timer firing and application-level restore remain to be verified on
the target machine. These isolated checks do not substitute for those steps.
