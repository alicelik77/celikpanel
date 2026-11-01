-- alpha.114 (owner decision 2026-10-31): the panel port can be changed from
-- the panel. One new table; no existing row or column is changed.
--
-- panel_port_changes: one change of the panel's own listen port. trial: the
-- panel also listens on the new port and waits for the owner to confirm
-- from it, until deadline_at; confirming: the Agent is writing the new port
-- into panel.env; confirmed: panel.env names the new port and the old one is
-- closed; rolled_back: the panel stays on the old port (cancelled, timed
-- out, restarted before confirmation, or a step failed). reason_code and
-- reason say why a change ended or what is still to do; firewall_note says
-- what the firewall step left to do, if anything.
CREATE TABLE panel_port_changes (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 8 AND 80),
    from_listen TEXT NOT NULL CHECK (length(from_listen) BETWEEN 2 AND 300),
    to_listen TEXT NOT NULL CHECK (length(to_listen) BETWEEN 2 AND 300),
    env_sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('trial', 'confirming', 'confirmed', 'rolled_back')),
    reason_code TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    firewall_note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    deadline_at TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX panel_port_changes_open ON panel_port_changes(status) WHERE status IN ('trial', 'confirming');
