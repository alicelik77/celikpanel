# guest (root) on A: the administrator's manual stop of mailbox id 3 as the panel stores it (lab only)
python3 - <<'P'
import sqlite3
c=sqlite3.connect("/var/lib/celikpanel/celikpanel.db",timeout=20)
c.execute("INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason, paused_detail, updated_at) VALUES (3, datetime('now'), 'manual', 'stopped by the administrator (lab)', datetime('now')) ON CONFLICT(email_account_id) DO UPDATE SET paused_at=excluded.paused_at, paused_reason='manual', paused_detail=excluded.paused_detail")
c.commit(); print(c.execute("select email_account_id, paused_at, paused_reason from mailbox_outgoing where paused_at is not null").fetchall())
P
