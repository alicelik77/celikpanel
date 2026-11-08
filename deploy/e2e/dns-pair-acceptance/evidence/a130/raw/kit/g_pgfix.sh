# guest (root): lab trial of a PostgreSQL restore into a non-empty database as the database's owner (lab DB only). Lab only.
DB=pg7_test_shop127; P=5433; cd /tmp
sudo -u postgres pg_dump --no-owner --no-privileges --dbname $DB --port $P > /tmp/lab-dump.sql
echo "dump lines $(wc -l < /tmp/lab-dump.sql)"
sudo -u postgres psql -p $P -d $DB -At -c "update lab_rows set v='changed-again'; create table lab_extra(x int); select v from lab_rows;"
cat > /tmp/lab-pre.sql <<'S'
DO $cp$ BEGIN EXECUTE format('SET ROLE %I', (SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database())); END $cp$;
DROP OWNED BY CURRENT_USER;
S
sudo -u postgres psql --set ON_ERROR_STOP=on --dbname $DB --port $P -c "$(sed -n 1p /tmp/lab-pre.sql)" -c "DROP OWNED BY CURRENT_USER" -f - < /tmp/lab-dump.sql 2>&1 | tail -4; echo "rc ${PIPESTATUS[0]}"
sudo -u postgres psql -p $P -d $DB -At -c "select v from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public';"
rm -f /tmp/lab-dump.sql /tmp/lab-pre.sql
