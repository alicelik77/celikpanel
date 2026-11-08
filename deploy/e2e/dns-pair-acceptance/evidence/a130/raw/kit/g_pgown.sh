# guest (root): the owner's own role, database and table in PostgreSQL 16 (lab), then the owner's apt-get remove. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
P=$(pg_lsclusters -h | awk '$1==16{print $3}'); echo "16 port $P"
cd /tmp
sudo -u postgres psql -p "$P" -v ON_ERROR_STOP=1 -At <<'S'
create role owner_app login;
create database owner_data owner owner_app;
S
sudo -u postgres psql -p "$P" -d owner_data -At -c "create table notes(t text); insert into notes values ('kept by the owner'); select count(*) from notes;"
DEBIAN_FRONTEND=noninteractive apt-get remove -y postgresql-16 2>&1 | tail -1
pg_lsclusters | cat
