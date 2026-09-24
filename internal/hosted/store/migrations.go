package store

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS accounts(id TEXT PRIMARY KEY,email_hash TEXT NOT NULL UNIQUE,email TEXT NOT NULL,created_at TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('active','deleted','deleting','restore_pending')),plan_id TEXT NOT NULL,plan_version INTEGER NOT NULL,stripe_customer_id TEXT UNIQUE);
CREATE TABLE IF NOT EXISTS brains(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),state TEXT NOT NULL CHECK(state IN ('allocating','ready','deleted')),is_default INTEGER NOT NULL DEFAULT 1,path_key TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,ready_at TEXT,deleted_at TEXT,UNIQUE(id,account_id));
CREATE INDEX IF NOT EXISTS brains_account_state ON brains(account_id,state);
CREATE UNIQUE INDEX IF NOT EXISTS brains_default_active ON brains(account_id) WHERE deleted_at IS NULL AND is_default=1;
CREATE TABLE IF NOT EXISTS client_credentials(id TEXT PRIMARY KEY,brain_id TEXT NOT NULL,account_id TEXT NOT NULL,prefix TEXT NOT NULL UNIQUE,verifier TEXT NOT NULL,scopes TEXT NOT NULL,generation INTEGER NOT NULL CHECK(generation>0),created_at TEXT NOT NULL,revoked_at TEXT,FOREIGN KEY(brain_id,account_id) REFERENCES brains(id,account_id));
CREATE TABLE IF NOT EXISTS login_tokens(id TEXT PRIMARY KEY,email TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,expires_at TEXT NOT NULL,consumed_at TEXT);
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),token_hash TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,expires_at TEXT NOT NULL,csrf_secret TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS usage_windows(account_id TEXT NOT NULL REFERENCES accounts(id),window_key TEXT NOT NULL,metric TEXT NOT NULL,committed INTEGER NOT NULL DEFAULT 0 CHECK(committed>=0),PRIMARY KEY(account_id,window_key,metric));
CREATE TABLE IF NOT EXISTS reservations(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),window_key TEXT NOT NULL,metric TEXT NOT NULL,amount INTEGER NOT NULL CHECK(amount>=0),operation_key TEXT,status TEXT NOT NULL,lease_expires_at TEXT,created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS reservation_released_age ON reservations(created_at) WHERE status='released';
CREATE INDEX IF NOT EXISTS reservation_open_expiry ON reservations(lease_expires_at) WHERE status='open';
CREATE INDEX IF NOT EXISTS reservation_replay ON reservations(account_id,metric,operation_key,created_at) WHERE status!='released';
CREATE UNIQUE INDEX IF NOT EXISTS reservation_operation ON reservations(account_id,window_key,metric,operation_key) WHERE operation_key IS NOT NULL AND status!='released';
CREATE TABLE IF NOT EXISTS checkout_attempts(account_id TEXT PRIMARY KEY REFERENCES accounts(id),id TEXT NOT NULL,price_id TEXT NOT NULL,session_id TEXT,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS stripe_events(id TEXT PRIMARY KEY,received_at TEXT NOT NULL,processed_at TEXT);
CREATE TABLE IF NOT EXISTS subscriptions(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id),price_id TEXT NOT NULL,status TEXT NOT NULL,current_period_start TEXT,current_period_end TEXT,cancel_at_period_end INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS audit_log(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT,actor TEXT NOT NULL,action TEXT NOT NULL,created_at TEXT NOT NULL,detail TEXT);
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(1,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

const migration2 = `
ALTER TABLE subscriptions ADD COLUMN plan_id TEXT NOT NULL DEFAULT 'free';
INSERT INTO schema_migrations(version,applied_at) VALUES(2,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

const migration3 = `
ALTER TABLE subscriptions ADD COLUMN grace_until TEXT;
INSERT INTO schema_migrations(version,applied_at) VALUES(3,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

// migration4 stores the durable operation journal. Delta and evidence payloads
// are serialized by the store layer; phase and ownership invariants remain
// enforced by SQLite so every writer observes the same uniqueness boundary.
const migration4 = `
CREATE TABLE operations(
 id TEXT PRIMARY KEY,
 account_id TEXT NOT NULL REFERENCES accounts(id),
 brain_id TEXT NOT NULL,
 client_key TEXT NOT NULL DEFAULT '',
 fingerprint TEXT NOT NULL DEFAULT '',
 deltas_json TEXT NOT NULL CHECK(json_valid(deltas_json) AND json_type(deltas_json)='array'),
 quota_period TEXT NOT NULL,
 phase TEXT NOT NULL CHECK(phase IN ('reserved','committed','released','pending_review')),
 source TEXT NOT NULL,
 evidence_kind TEXT NOT NULL DEFAULT '',
 evidence_ref TEXT NOT NULL DEFAULT '',
 canonical_entered_at TEXT,
 lease_expires_at TEXT NOT NULL,
 created_at TEXT NOT NULL,
 finalized_at TEXT,
 FOREIGN KEY(brain_id,account_id) REFERENCES brains(id,account_id)
);
CREATE UNIQUE INDEX operations_active_client_key
 ON operations(account_id,brain_id,client_key)
 WHERE client_key<>'' AND phase<>'released';
CREATE INDEX operations_expired_reserved
 ON operations(lease_expires_at,id) WHERE phase='reserved';
CREATE INDEX operations_pending_review
 ON operations(account_id,brain_id,created_at) WHERE phase='pending_review';
INSERT INTO schema_migrations(version,applied_at) VALUES(4,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

// OAuth records share the control database snapshot/restore boundary. Only
// secret digests are persisted; pending grants are independently one-time.
const migration5 = `
CREATE TABLE oauth_epochs(brain_id TEXT PRIMARY KEY REFERENCES brains(id),generation INTEGER NOT NULL);
CREATE TABLE oauth_clients(id TEXT PRIMARY KEY,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE TABLE oauth_consents(id TEXT PRIMARY KEY,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE TABLE oauth_codes(id TEXT PRIMARY KEY,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE TABLE oauth_grants(id TEXT PRIMARY KEY,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE TABLE oauth_tokens(id TEXT PRIMARY KEY,grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE TABLE oauth_refresh(id TEXT PRIMARY KEY,grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,record TEXT NOT NULL CHECK(json_valid(record)),expires_at TEXT NOT NULL);
CREATE INDEX oauth_grants_expiry ON oauth_grants(expires_at);
CREATE INDEX oauth_tokens_grant ON oauth_tokens(grant_id);
CREATE INDEX oauth_refresh_grant ON oauth_refresh(grant_id);
CREATE INDEX oauth_grants_client ON oauth_grants(json_extract(record,'$.client_id'),expires_at);
CREATE INDEX oauth_consents_client ON oauth_consents(json_extract(record,'$.client_id'),expires_at);
CREATE INDEX oauth_codes_client ON oauth_codes(json_extract(record,'$.client_id'),expires_at);
CREATE INDEX oauth_clients_expiry ON oauth_clients(expires_at);
CREATE INDEX oauth_consents_expiry ON oauth_consents(expires_at);
CREATE INDEX oauth_codes_expiry ON oauth_codes(expires_at);
CREATE INDEX oauth_tokens_expiry ON oauth_tokens(expires_at);
INSERT INTO schema_migrations(version,applied_at) VALUES(5,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

// migration6 preserves the exact provider evidence behind a grace deadline
// and the immutable parameters used to create a Checkout Session. Without
// these records, process-order and configuration changes could change billing
// outcomes after a retry or restart.
const migration6 = `
ALTER TABLE checkout_attempts ADD COLUMN request_version INTEGER NOT NULL DEFAULT 0 CHECK(request_version IN (0,1));
ALTER TABLE checkout_attempts ADD COLUMN request_body TEXT;
CREATE TRIGGER checkout_attempt_request_immutable
 BEFORE UPDATE OF id,request_version,request_body ON checkout_attempts
 WHEN NEW.id<>OLD.id OR NEW.request_version<>OLD.request_version OR NEW.request_body IS NOT OLD.request_body
 BEGIN SELECT RAISE(ABORT,'checkout request identity is immutable'); END;
CREATE TABLE billing_failures(
 account_id TEXT NOT NULL REFERENCES accounts(id),
 subscription_id TEXT NOT NULL,
 invoice_id TEXT NOT NULL,
 event_id TEXT NOT NULL,
 first_failed_at TEXT NOT NULL,
 PRIMARY KEY(subscription_id,invoice_id)
);
CREATE INDEX billing_failures_account ON billing_failures(account_id,subscription_id);
INSERT INTO schema_migrations(version,applied_at) VALUES(6,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`

// migration7 records which exact provider invoice established the current
// past-due grace window. A second failed invoice in the same billing period
// must not silently extend access.
const migration7 = `
ALTER TABLE subscriptions ADD COLUMN grace_invoice_id TEXT;
INSERT INTO schema_migrations(version,applied_at) VALUES(7,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
`
