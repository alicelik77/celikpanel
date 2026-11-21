-- 080: one name rule in the database (security release, 20 November 2026).
--
-- The owner's rule: a reseller or a customer never reaches anything of
-- another customer or of the server itself. A name a subscription of an
-- ordinary account (customer or reseller) receives must not equal, lie under
-- or lie above
--   * a name another account holds (hostname_reservations of a subscription
--     with a different owner: domains, their implicit www and mail names,
--     aliases), or
--   * a name of the server: a zone hosted here that no domain holds (the
--     infrastructure zones setup made, zones adopted from an earlier DNS
--     server, zones published here for a connected server), the stored
--     nameserver and mail host names, and the infrastructure zones setup
--     recorded.
-- A subscription owned by an administrator account may hold such a name on
-- purpose. The panel applies the same rule first, with the names only it can
-- read (the machine's and the panel certificate's host names, setup's draft
-- names, the webmail address) and with an answer that does not say who holds
-- a name (cmd/panel/hostname_claim.go).
--
-- Schema transition 79 -> 80: triggers only. No table, column or row is
-- added, changed or removed, so the migration cannot fail on a database that
-- already holds an overlap made before this rule. Each trigger fires only
-- when a name is newly given or changes hands (a new domain or alias, a
-- renamed or moved domain, a subscription with a new owner) and compares only
-- that name, so an existing overlap never breaks an unrelated write. Existing
-- overlaps are listed for the administrator, read only (Problems now).
--
-- Tek ad kurali veritabaninda: yalniz yeni verilen ya da el degistiren ad
-- karsilastirilir; var olan cakisma gecisi ve ilgisiz yazmalari bozmaz.


CREATE TRIGGER trg_domains_namespace_tenant_insert
BEFORE INSERT ON domains
WHEN COALESCE((SELECT u.role FROM subscriptions s JOIN users u ON u.id = s.owner_id
               WHERE s.id = NEW.subscription_id), '') <> 'admin'
AND (
    EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE s.owner_id IS NOT (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
          AND (r.hostname = NEW.name OR substr(NEW.name, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;

CREATE TRIGGER trg_domains_namespace_tenant_rename
BEFORE UPDATE OF name ON domains
WHEN NEW.name IS NOT OLD.name
AND COALESCE((SELECT u.role FROM subscriptions s JOIN users u ON u.id = s.owner_id
               WHERE s.id = NEW.subscription_id), '') <> 'admin'
AND (
    EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE r.domain_id <> NEW.id AND s.owner_id IS NOT (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
          AND (r.hostname = NEW.name OR substr(NEW.name, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;

CREATE TRIGGER trg_domains_namespace_tenant_move
BEFORE UPDATE OF subscription_id ON domains
WHEN (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id) IS NOT (SELECT owner_id FROM subscriptions WHERE id = OLD.subscription_id)
AND COALESCE((SELECT u.role FROM subscriptions s JOIN users u ON u.id = s.owner_id
               WHERE s.id = NEW.subscription_id), '') <> 'admin'
AND (
    EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE r.domain_id <> NEW.id AND s.owner_id IS NOT (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
          AND (r.hostname = NEW.name OR substr(NEW.name, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;

CREATE TRIGGER trg_domain_aliases_namespace_tenant_insert
BEFORE INSERT ON domain_aliases
WHEN COALESCE((SELECT u.role FROM domains d JOIN subscriptions s ON s.id = d.subscription_id
               JOIN users u ON u.id = s.owner_id WHERE d.id = NEW.domain_id), '') <> 'admin'
AND (
    EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE s.owner_id IS NOT (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id
                                 WHERE d2.id = NEW.domain_id)
          AND (r.hostname = NEW.alias OR substr(NEW.alias, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.alias OR substr(NEW.alias, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;

CREATE TRIGGER trg_domain_aliases_namespace_tenant_update
BEFORE UPDATE OF alias, domain_id ON domain_aliases
WHEN (NEW.alias IS NOT OLD.alias OR NEW.domain_id IS NOT OLD.domain_id)
AND COALESCE((SELECT u.role FROM domains d JOIN subscriptions s ON s.id = d.subscription_id
               JOIN users u ON u.id = s.owner_id WHERE d.id = NEW.domain_id), '') <> 'admin'
AND (
    EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE NOT (r.source_kind = 'alias' AND r.source_id = OLD.id) AND s.owner_id IS NOT (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id
                                 WHERE d2.id = NEW.domain_id)
          AND (r.hostname = NEW.alias OR substr(NEW.alias, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.alias OR substr(NEW.alias, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;

CREATE TRIGGER trg_subscriptions_namespace_tenant_owner
BEFORE UPDATE OF owner_id ON subscriptions
WHEN NEW.owner_id IS NOT OLD.owner_id
AND COALESCE((SELECT role FROM users WHERE id = NEW.owner_id), '') <> 'admin'
AND EXISTS (
    SELECT 1 FROM hostname_reservations mine
    JOIN domains md ON md.id = mine.domain_id
    WHERE md.subscription_id = NEW.id
      AND (
        EXISTS (
        SELECT 1 FROM hostname_reservations r
        JOIN domains d ON d.id = r.domain_id
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE d.subscription_id <> NEW.id AND s.owner_id IS NOT NEW.owner_id
          AND (r.hostname = mine.hostname OR substr(mine.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname OR substr(r.hostname, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
    )
        OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = mine.hostname OR substr(mine.hostname, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = mine.hostname OR substr(mine.hostname, -(length(v) + 1)) = '.' || v OR substr(v, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = mine.hostname OR substr(mine.hostname, -(length(v) + 1)) = '.' || v OR substr(v, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
    )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;
