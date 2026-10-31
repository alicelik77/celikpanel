-- alpha.114 (out-of-office reply per mailbox, Plesk parity). One new table;
-- no existing row is changed.
--
-- email_autoresponders: the reply the Panel last applied for one mailbox.
-- The reply itself is the mailbox's Sieve script that Dovecot runs (it keeps
-- working without the Panel); this row is what the page shows and what the
-- next save starts from. A mailbox without a row has no reply set by
-- CelikPanel. The row goes with its mailbox (ON DELETE CASCADE).
-- addresses is a JSON array of additional addresses ("[]" for none).
CREATE TABLE email_autoresponders (
    account_id INTEGER PRIMARY KEY REFERENCES email_accounts(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 800),
    message TEXT NOT NULL CHECK (length(CAST(message AS BLOB)) <= 8192),
    start_date TEXT CHECK (start_date IS NULL OR start_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    end_date TEXT CHECK (end_date IS NULL OR end_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    interval_days INTEGER NOT NULL DEFAULT 1 CHECK (interval_days BETWEEN 1 AND 30),
    addresses TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(addresses) AND json_type(addresses) = 'array'),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
