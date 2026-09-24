-- Proposed integration payload only; not an assigned or applied migration.
-- Integrator41/57 assigns migration number and applies with store tests.
-- Store the exact non-secret URL-encoded creation body, not reconstructed config.
ALTER TABLE checkout_attempts ADD COLUMN request_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE checkout_attempts ADD COLUMN request_body TEXT;

CREATE TRIGGER checkout_request_shape_insert BEFORE INSERT ON checkout_attempts
WHEN NOT ((NEW.request_version = 0 AND NEW.request_body IS NULL)
       OR (NEW.request_version = 1 AND NEW.request_body IS NOT NULL AND length(NEW.request_body) > 0))
BEGIN SELECT RAISE(ABORT, 'invalid checkout request shape'); END;

CREATE TRIGGER checkout_request_shape_update BEFORE UPDATE ON checkout_attempts
WHEN NOT ((NEW.request_version = 0 AND NEW.request_body IS NULL)
       OR (NEW.request_version = 1 AND NEW.request_body IS NOT NULL AND length(NEW.request_body) > 0))
BEGIN SELECT RAISE(ABORT, 'invalid checkout request shape'); END;

-- Session identity can be saved separately; the submitted payload cannot change.
-- Replacing an attempt requires an explicitly authorized transactional delete/insert.
CREATE TRIGGER checkout_request_immutable BEFORE UPDATE ON checkout_attempts
WHEN NEW.id = OLD.id AND
     (NEW.request_version IS NOT OLD.request_version OR NEW.request_body IS NOT OLD.request_body)
BEGIN SELECT RAISE(ABORT, 'checkout request is immutable'); END;

-- The historical UPSERT omits request columns and would inherit the old body.
-- Reject identity updates rather than accepting a valid-shaped stale payload.
CREATE TRIGGER checkout_attempt_identity_immutable BEFORE UPDATE ON checkout_attempts
WHEN NEW.id IS NOT OLD.id OR NEW.account_id IS NOT OLD.account_id
BEGIN SELECT RAISE(ABORT, 'replace checkout attempt with explicit delete and insert'); END;
