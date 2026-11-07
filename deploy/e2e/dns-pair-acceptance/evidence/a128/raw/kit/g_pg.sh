# guest (root): lab PostgreSQL probes. sql PORT DB B64SQL | find NAME | ppa. Lab only.
case "$1" in
sql) S=$(echo "$4" | base64 -d); cd /tmp; sudo -u postgres psql -p "$2" -d "$3" -At -c "$S" 2>&1 | head -20 ;;
find) for x in $(find /var/www/celikpanel /var/backups /var/lib/celikpanel /var/lib/celikpanel-agent-private -name "$2*" 2>/dev/null | head -3); do
        echo "file $x $(stat -c %s $x)"
        case $x in *.json) head -c 800 $x; echo;; *.tar.gz|*.tgz) tar -tzf $x 2>/dev/null | head -8; tar -xzOf $x --wildcards '*manifest*' '*meta*' '*.json' 2>/dev/null | head -c 1200; echo;; esac; done ;;
ppa) for f in /etc/phppgadmin/config.inc.php /usr/share/phppgadmin/conf/config.inc.php; do [ -f $f ] && { echo "== $f"; grep -nE "servers'?\]\[[0-9]+\]\['?(desc|host|port)" $f; grep -n "celikpanel" $f | head -5; }; done; ls -la /etc/phppgadmin 2>&1 | head ;;
esac
