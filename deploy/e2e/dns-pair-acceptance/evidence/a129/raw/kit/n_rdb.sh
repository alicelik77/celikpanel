# labnet (root): one remote database connection. usage: n_rdb.sh my|pg HOST USER DB tls|plain|wrong [PORT]
# (password on stdin; never printed). Prints the result line and the time it took. Lab only.
E=$1; H=$2; U=$3; DB=$4; M=$5; IFS= read -r PW
[ "$M" = wrong ] && PW="Wrong-${PW}x"
T0=$(date +%s%N)
case $E in
my)
  case $M in plain) SSL="--skip-ssl";; *) SSL="--ssl --skip-ssl-verify-server-cert";; esac
  out=$(MYSQL_PWD="$PW" timeout 15 mariadb --connect-timeout=8 -h "$H" -P ${6:-3306} -u "$U" $SSL -N -B "$DB" -e "select concat('ok ', @@hostname, ' ', ifnull((select variable_value from information_schema.session_status where variable_name='Ssl_cipher'),'-'))" 2>&1 | tail -1) ;;
pg)
  case $M in plain) SM=disable;; *) SM=require;; esac
  out=$(PGPASSWORD="$PW" PGCONNECT_TIMEOUT=8 timeout 15 psql -X -A -t "host=$H port=${6:-5432} dbname=$DB user=$U sslmode=$SM" -c "select 'ok ' || coalesce((select version from pg_stat_ssl where pid = pg_backend_pid())::text, '-')" 2>&1 | tail -1) ;;
esac
rc=$?
echo "  $E $M -> $(echo "$out" | cut -c1-200) [$(( ($(date +%s%N) - T0) / 1000000 )) ms]"
