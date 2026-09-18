package store

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS accounts(id TEXT PRIMARY KEY,email_hash TEXT NOT NULL UNIQUE,email TEXT NOT NULL,created_at TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('active','deleted')),plan_id TEXT NOT NULL,plan_version INTEGER NOT NULL,stripe_customer_id TEXT UNIQUE);
CREATE TABLE IF NOT EXISTS brains(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),state TEXT NOT NULL CHECK(state IN ('allocating','ready','deleted')),path_key TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,ready_at TEXT,deleted_at TEXT,UNIQUE(id,account_id));
CREATE INDEX IF NOT EXISTS brains_account_state ON brains(account_id,state);
CREATE UNIQUE INDEX IF NOT EXISTS brains_default_active ON brains(account_id) WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS client_credentials(id TEXT PRIMARY KEY,brain_id TEXT NOT NULL,account_id TEXT NOT NULL,prefix TEXT NOT NULL UNIQUE,verifier TEXT NOT NULL,scopes TEXT NOT NULL,generation INTEGER NOT NULL CHECK(generation>0),created_at TEXT NOT NULL,revoked_at TEXT,FOREIGN KEY(brain_id,account_id) REFERENCES brains(id,account_id));
CREATE TABLE IF NOT EXISTS login_tokens(id TEXT PRIMARY KEY,email TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,expires_at TEXT NOT NULL,consumed_at TEXT);
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),token_hash TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,expires_at TEXT NOT NULL,csrf_secret TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS usage_windows(account_id TEXT NOT NULL REFERENCES accounts(id),window_key TEXT NOT NULL,metric TEXT NOT NULL,committed INTEGER NOT NULL DEFAULT 0 CHECK(committed>=0),PRIMARY KEY(account_id,window_key,metric));
CREATE TABLE IF NOT EXISTS reservations(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),window_key TEXT NOT NULL,metric TEXT NOT NULL,amount INTEGER NOT NULL CHECK(amount>=0),operation_key TEXT,status TEXT NOT NULL,lease_expires_at TEXT,created_at TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS reservation_operation ON reservations(account_id,window_key,metric,operation_key) WHERE operation_key IS NOT NULL AND status!='released';
CREATE TABLE IF NOT EXISTS stripe_events(id TEXT PRIMARY KEY,received_at TEXT NOT NULL,processed_at TEXT);
CREATE TABLE IF NOT EXISTS subscriptions(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),price_id TEXT NOT NULL,status TEXT NOT NULL,current_period_start TEXT,current_period_end TEXT,cancel_at_period_end INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS audit_log(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT,actor TEXT NOT NULL,action TEXT NOT NULL,created_at TEXT NOT NULL,detail TEXT);
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(1,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`
