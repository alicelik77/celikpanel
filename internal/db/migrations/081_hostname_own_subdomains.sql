-- 081: a subdomain of one's own domain is an ordinary subdomain (fix round
-- after the alpha.142 lab, 21 November 2026).
--
-- Migration 080 refuses a name for a customer's or reseller's subscription
-- when it meets any name of the server or any name of another account. A
-- customer whose domain lies under a server zone (made before that rule: on
-- the lab, every customer's domain under the server's zone dr.test, which
-- the administrator's own subscription also holds as the main domain) could
-- therefore no longer add blog.<own domain>, although the refusal itself
-- says such a name can always be added.
--
-- For this exemption only, a name an administrator account holds (any kind:
-- a domain, an alias, their implicit www and mail names) is treated like a
-- server name. A server name, or an administrator's name, lets a name pass
-- when the receiving account already holds a domain or alias R
-- (hostname_reservations of kind primary or alias, never the implicit www
-- and mail names) with the name under R and R under it: it lies above a
-- domain the account already has and the new name stays inside that domain.
-- Everything else stays refused: the name equal to it, it under the name,
-- the name under it without such an R, it between R and the name (the
-- server's mail host name or an administrator's name inside the customer's
-- domain), and every overlap with an ordinary (non-administrator) account's
-- name exactly as in 080. The panel applies the same exemption first
-- (cmd/panel/hostname_claim.go, insideOwnDomainUnder).
--
-- Schema transition 80 -> 81: triggers only. The six triggers of 080 are
-- dropped and created again under the same names; no table, column or row is
-- added, changed or removed, so the migration cannot fail on a database that
-- already holds overlaps (a trigger compares only a name that is newly given
-- or changes hands). Migration 080 stays as published. Going back is the
-- pre-update snapshot restore, as for every migration.
--
-- Kendi alan adinin alt alan adi siradan bir alt alan adidir: sunucu adi ya
-- da yonetici hesabinin tuttugu ad, hesabin zaten tuttugu bir alan adinin
-- ustundeyse ve yeni ad o alan adinin icindeyse reddetmez. Baska siradan
-- hesaplarin adlari icin hicbir sey degismez. Yalniz tetikleyiciler degisir.


DROP TRIGGER IF EXISTS trg_domains_namespace_tenant_insert;
DROP TRIGGER IF EXISTS trg_domains_namespace_tenant_rename;
DROP TRIGGER IF EXISTS trg_domains_namespace_tenant_move;
DROP TRIGGER IF EXISTS trg_domain_aliases_namespace_tenant_insert;
DROP TRIGGER IF EXISTS trg_domain_aliases_namespace_tenant_update;
DROP TRIGGER IF EXISTS trg_subscriptions_namespace_tenant_owner;

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
          AND NOT (
              COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
              AND EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
              )
          )
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
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
          AND NOT (
              COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
              AND EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND own_r.domain_id <> NEW.id
                    AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
              )
          )
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
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
          AND NOT (
              COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
              AND EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND own_r.domain_id <> NEW.id
                    AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
              )
          )
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.name OR substr(NEW.name, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.name OR substr(NEW.name, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.name) + 1)) = '.' || NEW.name)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT owner_id FROM subscriptions WHERE id = NEW.subscription_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND own_r.domain_id <> NEW.id
                AND substr(NEW.name, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
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
          AND NOT (
              COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
              AND EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
              )
          )
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.alias OR substr(NEW.alias, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
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
          AND NOT (
              COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
              AND EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND NOT (own_r.source_kind = 'alias' AND own_r.source_id = OLD.id)
                    AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
              )
          )
    )
    OR EXISTS (
        SELECT 1 FROM pdns_domains z
        WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
          AND (z.name = NEW.alias OR substr(NEW.alias, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND NOT (own_r.source_kind = 'alias' AND own_r.source_id = OLD.id)
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
            WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND NOT (own_r.source_kind = 'alias' AND own_r.source_id = OLD.id)
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
    )
    OR EXISTS (
        SELECT 1 FROM (
            SELECT substr(key, 34) AS v FROM panel_settings
            WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
        ) WHERE v <> '' AND (v = NEW.alias OR substr(NEW.alias, -(length(v) + 1)) = '.' || v OR substr(v, -(length(NEW.alias) + 1)) = '.' || NEW.alias)
          AND NOT EXISTS (
              SELECT 1 FROM hostname_reservations own_r
              JOIN domains own_d ON own_d.id = own_r.domain_id
              JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
              WHERE own_s.owner_id = (SELECT s2.owner_id FROM domains d2 JOIN subscriptions s2 ON s2.id = d2.subscription_id WHERE d2.id = NEW.domain_id)
                AND own_r.source_kind IN ('primary', 'alias')
                AND NOT (own_r.source_kind = 'alias' AND own_r.source_id = OLD.id)
                AND substr(NEW.alias, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
          )
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
              AND NOT (
                  COALESCE((SELECT adm_u.role FROM users adm_u WHERE adm_u.id = s.owner_id), '') = 'admin'
                  AND EXISTS (
                      SELECT 1 FROM hostname_reservations own_r
                      JOIN domains own_d ON own_d.id = own_r.domain_id
                      JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                      WHERE own_s.owner_id = NEW.owner_id
                        AND own_r.source_kind IN ('primary', 'alias')
                        AND own_d.subscription_id <> NEW.id
                        AND substr(mine.hostname, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                        AND substr(own_r.hostname, -(length(r.hostname) + 1)) = '.' || r.hostname
                  )
              )
        )
        OR EXISTS (
            SELECT 1 FROM pdns_domains z
            WHERE NOT EXISTS (SELECT 1 FROM domains x WHERE x.name = z.name)
              AND (z.name = mine.hostname OR substr(mine.hostname, -(length(z.name) + 1)) = '.' || z.name OR substr(z.name, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
              AND NOT EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = NEW.owner_id
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND own_d.subscription_id <> NEW.id
                    AND substr(mine.hostname, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(z.name) + 1)) = '.' || z.name
              )
        )
        OR EXISTS (
            SELECT 1 FROM (
                SELECT rtrim(lower(trim(value)), '.') AS v FROM panel_settings
                WHERE key IN ('nameserver1', 'nameserver2', 'dns_peer_ns', 'mail_hostname')
            ) WHERE v <> '' AND (v = mine.hostname OR substr(mine.hostname, -(length(v) + 1)) = '.' || v OR substr(v, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
              AND NOT EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = NEW.owner_id
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND own_d.subscription_id <> NEW.id
                    AND substr(mine.hostname, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
              )
        )
        OR EXISTS (
            SELECT 1 FROM (
                SELECT substr(key, 34) AS v FROM panel_settings
                WHERE substr(key, 1, 33) = 'server_setup_infrastructure_zone:'
            ) WHERE v <> '' AND (v = mine.hostname OR substr(mine.hostname, -(length(v) + 1)) = '.' || v OR substr(v, -(length(mine.hostname) + 1)) = '.' || mine.hostname)
              AND NOT EXISTS (
                  SELECT 1 FROM hostname_reservations own_r
                  JOIN domains own_d ON own_d.id = own_r.domain_id
                  JOIN subscriptions own_s ON own_s.id = own_d.subscription_id
                  WHERE own_s.owner_id = NEW.owner_id
                    AND own_r.source_kind IN ('primary', 'alias')
                    AND own_d.subscription_id <> NEW.id
                    AND substr(mine.hostname, -(length(own_r.hostname) + 1)) = '.' || own_r.hostname
                    AND substr(own_r.hostname, -(length(v) + 1)) = '.' || v
              )
        )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'hostname namespace conflict: another account or this server holds this name, a name above it or a name under it');
END;
