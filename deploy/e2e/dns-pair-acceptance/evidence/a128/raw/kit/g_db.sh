# guest (root): lab load and reads inside the database engines. usage: g_db.sh my|pg DB BASE64SQL [PGPORT]. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
E=$1; DB=$2; SQL=$(echo "$3" | base64 -d)
case $E in
my) mariadb -N -B "$DB" -e "$SQL" 2>&1 | head -40 ;;
pg) cd /tmp; runuser -u postgres -- psql -X -A -t -p ${4:-5432} -d "$DB" -c "$SQL" 2>&1 | head -40 ;;
esac
