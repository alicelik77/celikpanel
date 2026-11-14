-- 078 (alpha.134, docs/CERTIFICATE-COVERAGE.md): which name a domain's mail
-- programs are told.
--
-- Until alpha.134 one switch ("secure mail with this certificate",
-- ssl_certificates.secure_mail) did two things: mail served the domain's
-- certificate for mail.<domain>, and mail programs were told mail.<domain>
-- instead of the server's mail name. From alpha.134 a certificate that covers
-- mail.<domain> is served for that name by default, and what programs are
-- told stays as it was. mail_clients_use_own_name keeps the old choice for
-- the domains that made it: 1 tells mail.<domain> (while mail serves the
-- domain's certificate for it), 0 tells the server's mail name.
--
-- Additive: one column, set to 1 exactly for the domains whose active
-- certificate secured mail before this release, so no program is told a
-- different name after the update. The release before this one refuses a
-- ledger that carries this entry; going back is the pre-update snapshot
-- restore, as for every migration (D-025).
--
-- Posta programlarına hangi adın söylendiği. Eski seçimi yapan alan adları 1
-- ile işaretlenir; hiçbir programa güncellemeden sonra başka ad söylenmez.

ALTER TABLE domains ADD COLUMN mail_clients_use_own_name INTEGER NOT NULL DEFAULT 0
    CHECK (mail_clients_use_own_name IN (0, 1));

UPDATE domains SET mail_clients_use_own_name = 1
WHERE id IN (
    SELECT domain_id FROM ssl_certificates
    WHERE status = 'active' AND secure_mail = 1
);
