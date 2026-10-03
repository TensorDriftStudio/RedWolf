package sqlite

const schemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    vendor TEXT NOT NULL,
    model TEXT NOT NULL,
    serial_number TEXT NOT NULL,
    firmware_mode TEXT NOT NULL,
    bios_version TEXT NOT NULL,
    status TEXT NOT NULL,
    boot_mac TEXT NOT NULL,
    cpu_json TEXT NOT NULL,
    memory_json TEXT NOT NULL,
    storage_json TEXT NOT NULL,
    nics_json TEXT NOT NULL,
    bmc_json TEXT NOT NULL,
    provisioning_json TEXT,
    discovered_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS node_macs (
    mac TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_boot_mac ON nodes(boot_mac);
CREATE INDEX IF NOT EXISTS idx_nodes_serial_number ON nodes(serial_number);
CREATE INDEX IF NOT EXISTS idx_node_macs_node_id ON node_macs(node_id);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    email TEXT NOT NULL,
    role TEXT NOT NULL,
    source TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    last_login_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
`

