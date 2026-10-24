-- alpha.107 (D-038 §16 and §6). Two additive columns; no row is changed.
--
-- audit_logs.details: what an audited decision was about, as one JSON object
-- (component, package, version from and to, what was confirmed: notable
-- installs, an unsupported version, a move). NULL for every row written
-- before this release and for actions that carry nothing beyond their name.
--
-- sites.app_run_as: which account a Node site's application runs as. 'site'
-- is the site's own user (every application created from this release on);
-- NULL is the shared web user, as every application created before it, so
-- an upgrade moves nothing (the owner moves an application explicitly, one
-- at a time, with proof and rollback).
ALTER TABLE audit_logs ADD COLUMN details TEXT CHECK (details IS NULL OR (length(details) <= 8192 AND json_valid(details)));
ALTER TABLE sites ADD COLUMN app_run_as TEXT CHECK (app_run_as IS NULL OR app_run_as IN ('site', 'web'));
