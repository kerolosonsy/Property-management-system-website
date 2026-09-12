#!/usr/bin/env node
// Keep the database password out of process arguments for both backup and restore.
const { spawnSync } = require('node:child_process');
const command = process.argv[2];
if (!['pg_dump', 'pg_restore'].includes(command)) {
    console.error('Expected pg_dump or pg_restore.');
    process.exit(2);
}
let connection;
let password;
try {
    connection = new URL(process.env.PMS_DATABASE_OWNER_URL);
    if (!['postgres:', 'postgresql:'].includes(connection.protocol)) throw new Error();
    password = decodeURIComponent(connection.password);
} catch {
    console.error('PMS_DATABASE_OWNER_URL must be a PostgreSQL URL.');
    process.exit(2);
}
connection.password = '';
// A password may also be supplied as a libpq URI query parameter.
const queryPassword = connection.searchParams.get('password');
connection.searchParams.delete('password');
const child = spawnSync(command, ['--dbname', connection.toString(), ...process.argv.slice(3)], {
    stdio: 'inherit',
    env: { ...process.env, PGPASSWORD: queryPassword ?? password, PGCONNECT_TIMEOUT: '15' },
});
if (child.error) console.error(`Could not start ${command}. Check the PostgreSQL client installation.`);
process.exit(child.status ?? 1);
