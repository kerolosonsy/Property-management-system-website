# Deployment guide

An Arabic step-by-step version of the one-command procedure is in
[`docs/SETUP-AR.md`](SETUP-AR.md) — دليل التشغيل بالعربية.

## One-command local and LAN server

From a repository checkout, run the command for this machine:

```bash
# macOS / Linux
bash scripts/setup.sh
```

```powershell
# Windows (Windows PowerShell 5.1)
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1
```

These commands auto-select the database. To choose explicitly, use:

```bash
# macOS / Linux
bash scripts/setup.sh --db=native
bash scripts/setup.sh --db=docker
```

```powershell
# Windows (Windows PowerShell 5.1)
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1 --db=native
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1 --db=docker
```

When `--db` is omitted, setup chooses native PostgreSQL only when a locally
installed server is already running and reachable; otherwise it chooses Docker.
It prints the selected mode and reason during `Preflight`. `--db=docker` keeps
the previous Docker Compose path. `--db=native` never checks for or starts
Docker.

On macOS or Linux, `make setup` remains the equivalent no-argument command and
therefore uses auto-detection. Both scripts also accept `--no-start` (configure
and build without starting the API) and `--skip-deps` (verify dependencies but
leave their installation to the operator).

### Native PostgreSQL mode

Native mode requires PostgreSQL major version 17. On macOS setup installs
`postgresql@17` with Homebrew and manages it with `brew services`. On supported
Linux package managers it installs the version-17 server package, initializes
the distribution's cluster when necessary, and enables and starts its systemd
or OpenRC service. On Debian and Ubuntu, whose archives ship an older server
(jammy 14, noble 16), setup adds the official PostgreSQL apt repository (PGDG)
for the detected release, in the deb822 format documented on postgresql.org,
and installs `postgresql-17` from it. If another server major is installed or
the reachable server is not version 17, setup stops instead of attempting an
upgrade or downgrade.

The native server is configured to listen only on `127.0.0.1:5432`. Setup uses
the current user's Homebrew socket access on macOS and `sudo -u postgres psql`
on Linux to create the `pms` database, make `pms_owner` its owner, and keep
`pms_owner` as a login superuser. Docker is not a native-mode prerequisite.

On Windows, an unattended official PostgreSQL install cannot safely supply and
retain the required `postgres` bootstrap password without putting a secret in a
process argument. If PostgreSQL 17 is absent, setup stops with the exact
operator-run `winget` or Chocolatey installation command. After installation,
run setup from a PowerShell session whose `PGPASSWORD` was populated by
`Get-Credential`, as instructed by the error message. Setup then enables and
starts the PostgreSQL 17 Windows service, restricts its listener, and creates the
same roles and database. This Windows-native path has not been exercised here.

### Generated database passwords

For an empty or absent `PMS_DATABASE_OWNER_URL` or `PMS_DATABASE_APP_URL`, setup
generates a separate 48-character hexadecimal password and writes it only as the
password component of that DSN in `.env`. It never prints either password. After
migrations, it applies the generated credentials to `pms_owner` and `pms_app`
through SQL on standard input, so the secret is not a command-line argument.

An existing DSN is changed only when its password is exactly
`dev_only_owner_pw` or `dev_only_app_pw`. Setup then applies a newly generated
password, rewrites only that DSN, and prints one rotation notice. Any other
password is treated as operator-managed and is left unchanged without a repeated
warning. A second successful setup run therefore does not rotate either role.
All other non-empty `.env` values retain the existing never-overwrite behavior;
in particular, setup never replaces an existing `PMS_KEK`.

The installer detects the host, installs missing supported packages, fills empty
`.env` values while preserving non-empty operator values, rotates only the two
exact legacy database defaults described above, creates or reuses a self-signed
certificate, starts PostgreSQL, applies migrations, creates or resets only the
`admin` account, builds both applications, and starts the Go server. The new
certificate covers localhost, the hostname, and the primary LAN IPv4 address.
The Go server serves the Angular bundle and API over the same HTTPS listener.
The generated administrator password is stored only in `.env`; the installer
never prints it. Preserve and back up `PMS_KEK` before doing any maintenance
involving `.env`. If a distribution package cannot satisfy Go 1.27 or Node 22,
setup stops and names the upstream installation step rather than accepting the
old version.

The self-signed certificate is appropriate for a trusted local network after
each client explicitly trusts it. It is not a public production certificate.
Use `--no-start` when a service manager will own the process.

Verification status for this revision: the offline build and Bash syntax gates
were exercised on macOS with Apple silicon. Neither installer was executed. The
Linux package-manager paths, Windows PowerShell 5.1 entry point, Windows service
management, and Windows-native PostgreSQL bootstrap are written but unverified
on their target operating systems.

## Manual production deployment

The remaining guide is the fallback procedure and the recommended shape for a
real single-host Linux installation. Replace `pms.example.com` and the example
paths with values for the target host. Run application processes as an
unprivileged `pms` account. The repository still does not provide a production
application container image or CI gate. The binding verification procedures are
the manual steps in each feature's `specs/*/quickstart.md`.

## 1. Dependencies

The following versions are the tested toolchain for this revision:

| Dependency | Version | Purpose and failure mode |
| --- | --- | --- |
| Go | 1.27.0 | Builds the API, migration command, and administrator tool. It is not needed after the binaries are built. |
| Node.js | 22.23.0 | Builds the Angular application. It is not needed after the static files are built. |
| npm | 10.9.8 | Installs the locked web dependencies and runs Angular 22.1.6. |
| PostgreSQL | 17.x | Required runtime database. The application cannot start or serve data without it. |
| Tesseract | 5.5.2, including `ara` | Reads Arabic text from supported images and scanned PDF pages. If Tesseract or its Arabic data is absent, affected documents are deliberately marked `not_eligible`; upload and the rest of the application continue to work. |
| Poppler | 26.08.0 (`pdftotext`, `pdftoppm`, and `pdfinfo`) | Inspects PDFs, reads embedded text, and pipes rendered pages to OCR. If these programs are absent, PDF extraction is deliberately marked `not_eligible`; the attachment remains stored and usable. |
| OpenSSL | 3.x | Generates random secrets and TLS material. |
| Git | 2.45 or later | Retrieves and updates the source tree. It is unnecessary if release files arrive by another controlled mechanism. |
| nginx | 1.26 or later | Recommended static-file server and same-origin reverse proxy. Another TLS-capable reverse proxy can be used instead. |

On Debian or Ubuntu, the system packages for the extraction tools are normally
named `tesseract-ocr`, `tesseract-ocr-ara`, and `poppler-utils`. Install Go 1.27.0
and Node 22.23.0 from their upstream distributions if the operating-system
packages provide older versions. Verify the installation:

```bash
go version
node --version
npm --version
psql --version
tesseract --version
tesseract --list-langs | grep -x 'ara'
pdftotext -v
pdftoppm -v
pdfinfo -v
```

Create the service account and installation directories:

```bash
sudo useradd --system --home /var/lib/pms --create-home --shell /usr/sbin/nologin pms
sudo install -d -o pms -g pms -m 0750 /opt/pms /opt/pms/releases
sudo install -d -o root -g pms -m 0750 /etc/pms /etc/pms/tls
sudo install -d -o pms -g pms -m 0700 /var/lib/pms/attachments
sudo install -d -o root -g root -m 0755 /var/www/pms
```

The attachment directory must be outside the source repository. The server
refuses to start if it is missing, is not writable, is relative, or is inside
the repository working tree. Keep the directory mode at `0700`; stored blobs
are created without a descriptive filename or extension and should remain
readable only by the service account.

## 2. Environment and secrets

Generate independent database passwords and the encryption master key:

```bash
openssl rand -hex 24
openssl rand -hex 24
openssl rand -base64 32
```

The first two URL-safe outputs are the database-role passwords. The third output
is `PMS_KEK`; it must decode to exactly 32 bytes.

**`PMS_KEK` is the master encryption key. If it is lost, every encrypted
attachment and every sensitive custom-field value is permanently unreadable.**
There is no recovery mechanism. Back it up in a secrets system or offline
encrypted medium separately from the database and separately from the
attachment store. Restrict access to the application service and the small
number of operators who perform a full restore.

Create `/etc/pms/api.env` with mode `0640`, owned by `root:pms`:

```dotenv
PMS_DATABASE_APP_URL='postgres://pms_app:URL_ENCODED_PASSWORD@127.0.0.1:5432/pms?sslmode=require'
PMS_SESSION_COOKIE_NAME='pms_session'
PMS_TLS_CERT_PATH='/etc/pms/tls/server.crt'
PMS_TLS_KEY_PATH='/etc/pms/tls/server.key'
PMS_LISTEN_ADDR='127.0.0.1:8443'
PMS_ATTACHMENT_STORE='/var/lib/pms/attachments'
PMS_ATTACHMENT_MAX_BYTES='52428800'
PMS_EXTRACT_TEXT_MAX_BYTES='262144'
PMS_KEK='BASE64_OF_32_RANDOM_BYTES'
```

Create a separate root-only `/etc/pms/owner.env` with mode `0600` for offline
database work. Never expose this file to the running API:

```dotenv
PMS_DATABASE_OWNER_URL='postgres://pms_owner:URL_ENCODED_PASSWORD@127.0.0.1:5432/pms?sslmode=require'
```

The complete environment surface is:

| Variable | Required | Meaning |
| --- | --- | --- |
| `PMS_DATABASE_APP_URL` | API | PostgreSQL URL for `pms_app`, the restricted runtime role. |
| `PMS_DATABASE_OWNER_URL` | migration and administrator tools only | PostgreSQL URL for `pms_owner`. Do not put it in `api.env` or the systemd service. |
| `PMS_SESSION_COOKIE_NAME` | optional | Session cookie name; defaults to `pms_session`. Sessions use opaque random database tokens, so there is no separate session-signing secret. |
| `PMS_TLS_CERT_PATH` | API | PEM certificate chain for the API TLS listener. |
| `PMS_TLS_KEY_PATH` | API | PEM private key matching the API certificate. |
| `PMS_LISTEN_ADDR` | optional | TLS listen address; defaults to `:8443`. Use `127.0.0.1:8443` behind a local proxy. |
| `PMS_WEB_DIST` | optional | Absolute Angular bundle directory served by the Go process. Leave empty for the existing API-only/nginx deployment. |
| `PMS_ATTACHMENT_STORE` | API | Absolute, writable directory outside the repository for encrypted attachment blobs. |
| `PMS_ATTACHMENT_MAX_BYTES` | optional | Maximum upload size in bytes; defaults to 52,428,800 (50 MiB). |
| `PMS_EXTRACT_TEXT_MAX_BYTES` | optional | Maximum stored extracted text in bytes; defaults to 262,144 (256 KiB). |
| `PMS_KEK` | API | Base64 encoding of exactly 32 random bytes; the irreplaceable encryption master key. |
| `SEED_USERNAME` | optional Make variable | Username used by `make seed-admin`; defaults to `admin`. |
| `PMS_ADMIN_PASSWORD` | setup scripts only | Administrator bootstrap/reset input kept in `.env`; never pass its value on a command line. |
| `DEV_ADMIN_PASSWORD` | development only | Input for `make reset-admin-dev`. Do not set or use it on a deployed host. |

Connection-string passwords must be URL-encoded. Do not paste an unencoded
password containing `@`, `:`, `/`, `?`, or `#` into a PostgreSQL URL.

## 3. TLS certificates

Obtain a certificate for the public hostname from the site's certificate
authority. Install the full chain and private key:

```bash
sudo install -o root -g pms -m 0644 fullchain.pem /etc/pms/tls/server.crt
sudo install -o root -g pms -m 0640 privkey.pem /etc/pms/tls/server.key
```

The Go API always uses TLS, even when nginx reaches it through the loopback
interface. The certificate must be valid for the hostname configured by
`proxy_ssl_name` below. `make certs` creates a self-signed localhost certificate
for development only; do not use it as public server identity.

## 4. Database roles and migrations

Initialize PostgreSQL 17 according to the operating system's documentation,
enable TLS for local TCP connections, and require password authentication in
`pg_hba.conf`. Then open `psql` as the PostgreSQL superuser:

```sql
CREATE ROLE pms_owner LOGIN;
CREATE ROLE pms_app LOGIN;
\password pms_owner
\password pms_app
CREATE DATABASE pms OWNER pms_owner;
```

Pre-create `pms_app` with its strong password before running migrations. This
is important because migration `0004_roles_and_grants.sql` creates a development
role only when the role does not already exist. The migration grants the runtime
role normal access to business tables but only `SELECT, INSERT` on `audit_log`.
Never grant `UPDATE`, `DELETE`, ownership, or a broader default privilege on
`audit_log` to `pms_app`.

Install the source, dependencies, and generated clients, then build:

```bash
sudo -u pms git clone YOUR_REPOSITORY_URL /opt/pms/releases/INITIAL_RELEASE
sudo -u pms ln -s /opt/pms/releases/INITIAL_RELEASE /opt/pms/current
cd "/opt/pms/current/web"
npm ci
cd "/opt/pms/current"
make generate
make build
```

Apply migrations with owner credentials only:

```bash
cd "/opt/pms/current"
set -a
. "/etc/pms/owner.env"
set +a
make migrate
```

Confirm the audit-log boundary directly:

```sql
SELECT privilege_type
FROM information_schema.role_table_grants
WHERE grantee = 'pms_app' AND table_name = 'audit_log'
ORDER BY privilege_type;
```

The result must contain only `INSERT` and `SELECT`.

Seed the first administrator from an interactive root shell. The command asks
twice for a password of at least 12 characters without echoing it:

```bash
cd "/opt/pms/current"
set -a
. "/etc/pms/owner.env"
set +a
SEED_USERNAME='admin' make seed-admin
```

## 5. Run the services

Install `/etc/systemd/system/pms-api.service`:

```ini
[Unit]
Description=Property management API
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=pms
Group=pms
WorkingDirectory=/opt/pms/current/api
EnvironmentFile=/etc/pms/api.env
ExecStart=/opt/pms/current/api/bin/pms-api
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadOnlyPaths=/opt/pms/current /etc/pms
ReadWritePaths=/var/lib/pms/attachments

[Install]
WantedBy=multi-user.target
```

Install the built Angular files under nginx, without making the source tree
readable by the web-server account:

```bash
sudo find /var/www/pms -mindepth 1 -delete
sudo cp -a /opt/pms/current/web/dist/web/browser/. /var/www/pms/
sudo chown -R root:root /var/www/pms
```

A minimal same-origin server block is:

```nginx
server {
    listen 443 ssl http2;
    server_name pms.example.com;

    ssl_certificate /etc/pms/tls/server.crt;
    ssl_certificate_key /etc/pms/tls/server.key;

    root /var/www/pms;
    index index.html;

    location /api/v1/ {
        proxy_pass https://127.0.0.1:8443;
        proxy_ssl_server_name on;
        proxy_ssl_name pms.example.com;
        proxy_ssl_verify on;
        proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

Validate and start both services:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pms-api.service
sudo nginx -t
sudo systemctl reload nginx.service
sudo systemctl status pms-api.service --no-pager
sudo journalctl -u pms-api.service -n 100 --no-pager
```

The API startup log reports whether Tesseract, Arabic language data, and
Poppler were detected. Missing extraction tools do not prevent startup.

## 6. Backup and restore

The database stores attachment metadata and encryption material while
`PMS_ATTACHMENT_STORE` holds the encrypted bodies. Back up the database and
attachment store as one matched set: neither is a usable attachment backup
without the other. Back up `PMS_KEK` separately as described above.

For a consistent single-host backup, prevent writes while taking both copies:

```bash
sudo systemctl stop pms-api.service
sudo -u postgres pg_dump --format=custom --file=/srv/backup/pms-db.dump pms
sudo tar --create --gzip --file=/srv/backup/pms-attachments.tar.gz \
  --directory=/var/lib/pms attachments
sudo sha256sum /srv/backup/pms-db.dump /srv/backup/pms-attachments.tar.gz \
  > /srv/backup/SHA256SUMS
sudo systemctl start pms-api.service
```

Copy the database dump and attachment archive together to backup storage. Copy
the KEK backup to a different protected location. Record which KEK version and
which two data files belong to the same backup event. Test restoration regularly.

To restore onto an empty installation:

1. Stop `pms-api.service`.
2. Recreate the `pms_owner` and `pms_app` roles with new strong passwords.
3. Create an empty `pms` database owned by `pms_owner`.
4. Restore the database with `pg_restore --clean --if-exists --no-owner --role=pms_owner --dbname=pms /srv/backup/pms-db.dump`.
5. Remove the empty attachment directory and extract the matching archive under `/var/lib/pms`; restore ownership `pms:pms` and directory mode `0700`.
6. Restore the exact matching `PMS_KEK` into `/etc/pms/api.env`, update both database URLs, and recheck file permissions.
7. Start the API and perform the feature quickstart checks, including reading a sensitive field and opening an attachment.

Restoring only the database, only the attachment directory, or a different KEK
does not constitute a valid restore.

## 7. Upgrade

1. Read the release notes and all new migration files.
2. Take and verify a matched database, attachment-store, and KEK backup.
3. Stop `pms-api.service` so no writes occur during migration and file switch.
4. Fetch the new source into a new release directory under `/opt/pms`.
5. Run `npm ci`, `make generate`, `make check`, and the relevant manual `specs/*/quickstart.md` procedures.
6. Load `/etc/pms/owner.env` and run `make migrate` from the new release.
7. Atomically point `/opt/pms/current` at the new release and copy `web/dist/web/browser` into `/var/www/pms` as shown above.
8. Restart the API and reload nginx.
9. Check the API process start time, logs, health, sign-in, and a representative attachment read.

The restart in step 8 is mandatory. A rebuild does not replace a binary that is
already running in memory. A stale running binary after a successful rebuild has
caused real deployment failures in this project.

```bash
sudo systemctl restart pms-api.service
sudo systemctl reload nginx.service
sudo systemctl show pms-api.service --property=MainPID --property=ExecMainStartTimestamp
sudo journalctl -u pms-api.service -n 100 --no-pager
```

## 8. Troubleshooting

- **The source changed but behavior did not:** confirm that `make build` produced
  `api/bin/pms-api`, deploy that exact file, and restart `pms-api.service`.
  Compare `ExecMainStartTimestamp` with the deployment time. Rebuilding alone
  never changes the already running process.
- **The API refuses to start:** inspect `journalctl`. Check the database URL,
  certificate and key paths, absolute attachment-store path and permissions,
  and that `PMS_KEK` is valid base64 decoding to 32 bytes.
- **PDF extraction is `not_eligible`:** run `pdftotext -v`, `pdftoppm -v`, and
  `pdfinfo -v` as the `pms` user. This state is deliberate when Poppler is absent.
- **Image or scanned-PDF extraction is `not_eligible`:** run `tesseract
  --list-langs` as the `pms` user and confirm an exact `ara` entry. The deliberate
  degradation preserves the encrypted attachment without failing its upload.
- **Attachments exist in the database but cannot be opened:** verify that the
  matching attachment-store backup and the matching KEK were restored. Database
  rows alone do not contain attachment bodies.
- **Sensitive values and all attachments fail to decrypt:** stop the service and
  verify the KEK version. Do not generate a replacement key; doing so cannot
  recover old data and can make diagnosis harder.
- **The audit role check shows broader grants:** stop the API, revoke the excess
  privileges, and investigate before restarting. `pms_app` must have only
  `SELECT, INSERT` on `audit_log`.
- **The web application returns 404 after browser refresh:** for one-command
  setup, confirm `PMS_WEB_DIST` names the built directory containing
  `index.html`; for nginx, confirm it uses `try_files $uri $uri/ /index.html`.
