-- 073 (alpha.129): a resumed mailbox starts a
-- fresh spike window.
--
-- alpha.128 skipped the whole UTC hour of a password change when it judged
-- a mailbox for the security pause, because mail_outgoing_hourly counts the
-- whole hour and the messages that led to the pause are in it. A mailbox
-- that was reopened and abused again in the same hour was therefore never
-- paused again (alpha.128 lab, round 2). The pause now judges the hour of
-- the password change by what was sent after it: resume_base_recipients and
-- resume_base_refusals are that hour's accepted recipients and limit
-- refusals as Postfix's log told them when the pause ended (the hour of
-- resumed_at). Hours before it are still not judged again; later hours are
-- judged in full.
--
-- Additive only: two columns with a default of 0 on mailbox_outgoing. No
-- existing row or value changes; a row written by alpha.128 reads 0, which
-- is what alpha.128 meant for hours after its resume. The release before
-- this one refuses a ledger that carries this entry; going back is the
-- pre-update snapshot restore, as for every migration (D-025). Nothing a
-- hosted service reads is here: the pause itself is postfwd's paused.list
-- on the server (D-022).
--
-- 073 (alpha.129): parolası değiştirilen posta kutusu yeni bir ani artış
-- penceresiyle başlar; değişikliğin olduğu saat yalnız ondan sonra
-- gönderilenlerle değerlendirilir. Yalnız ekleme: iki sütun, varsayılan 0.

ALTER TABLE mailbox_outgoing ADD COLUMN resume_base_recipients INTEGER NOT NULL DEFAULT 0 CHECK (resume_base_recipients >= 0);
ALTER TABLE mailbox_outgoing ADD COLUMN resume_base_refusals INTEGER NOT NULL DEFAULT 0 CHECK (resume_base_refusals >= 0);
