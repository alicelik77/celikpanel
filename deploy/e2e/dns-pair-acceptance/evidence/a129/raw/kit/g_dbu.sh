# guest (root): run SQL as a database user over TCP to 127.0.0.1 (password on stdin, never printed).
# usage: g_dbu.sh my|pg USER DB BASE64SQL [PGPORT]. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
E=$1; U=$2; DB=$3; SQL=$(echo "$4" | base64 -d); IFS= read -r PW
case $E in
my) MYSQL_PWD="$PW" mariadb -u "$U" -N -B "$DB" -e "$SQL" 2>&1 | head -8 ;;
pg) cd /tmp; PGPASSWORD="$PW" psql -X -A -t -h 127.0.0.1 -p ${5:-5432} -U "$U" -d "$DB" -c "$SQL" 2>&1 | head -8 ;;
esac
