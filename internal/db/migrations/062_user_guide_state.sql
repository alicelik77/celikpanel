-- 062 (alpha.119): the first-login tour's state, one row per person
-- (docs/guide/FORMAT.md; web/src/help/guide/tourState.ts). Additive only: one
-- new table and one trigger; no existing table, row or value changes. The
-- release before this one refuses a ledger that carries entry 62; going back
-- is the pre-update snapshot restore, as for every migration.
--
-- A person without a row has not seen the tour ("pending"). The row records
-- the tour version they last finished or skipped, so a later version is
-- offered again and never forced. It is the person's own preference, read and
-- written only by them (GET/PUT /api/v1/me/guide-state); it holds no secret
-- and nothing of the server depends on it.
--
-- user_id is a plain integer without REFERENCES (project note in 004). The
-- trigger removes the row with its user, so a new account that receives a
-- reused id starts with the tour pending.
CREATE TABLE user_guide_state (
    user_id INTEGER PRIMARY KEY,
    tour_version INTEGER NOT NULL DEFAULT 0 CHECK (tour_version BETWEEN 0 AND 1000),
    tour_state TEXT NOT NULL DEFAULT 'pending' CHECK (tour_state IN ('pending', 'completed', 'skipped')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TRIGGER user_guide_state_user_deleted
AFTER DELETE ON users
BEGIN
    DELETE FROM user_guide_state WHERE user_id = OLD.id;
END;
