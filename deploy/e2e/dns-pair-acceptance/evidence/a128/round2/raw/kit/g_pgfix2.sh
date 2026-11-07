# guest (root): lab trial of the atomic restore (one transaction) on the lab DB. Lab only.
DB=pg7_test_shop127; P=5433; cd /tmp
Q() { sudo -u postgres psql -p $P -d $DB -At -c "$1" 2>&1; }
PRE=$(cat <<'S'
DO $celikpanel_restore$
DECLARE
  owner_name name := (SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database());
BEGIN
  IF NOT (SELECT rolsuper FROM pg_roles WHERE rolname = owner_name) THEN
    EXECUTE format('SET ROLE %I', owner_name);
    EXECUTE 'DROP OWNED BY CURRENT_USER';
  END IF;
END
$celikpanel_restore$
S
)
Q "update lab_rows set v='before-dump';"
sudo -u postgres pg_dump --no-owner --no-privileges --dbname $DB --port $P > /tmp/lab-dump.sql
Q "update lab_rows set v='changed-after-dump'; insert into lab_rows values ('extra');"
echo "-- 2. a restore into the non-empty database, one transaction"
sudo -u postgres psql --set ON_ERROR_STOP=on --dbname $DB --port $P --single-transaction --command "$PRE" --file - < /tmp/lab-dump.sql 2>&1 | tail -2; echo "rc ${PIPESTATUS[0]}"
Q "select v from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public' order by 1;"
rm -f /tmp/lab-dump.sql /tmp/lab-dump2.sql
