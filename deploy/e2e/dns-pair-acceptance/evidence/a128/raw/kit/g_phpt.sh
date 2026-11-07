# guest (root): the customer's own files in the site (lab load standing for an FTP upload) and the site's answers.
# usage: g_phpt.sh DOMAIN put|ini|sleep SECONDS|pool|cleanup. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
D=$1; M=$2
V=$(ls /etc/nginx/sites-enabled/$D.conf 2>/dev/null || grep -RlE "server_name[^;]*[[:space:]]$D[ ;]" /etc/nginx/sites-enabled/ /etc/nginx/conf.d/ /etc/nginx/http.d/ 2>/dev/null | head -1)
R=$(grep -E "^\s*root " "$V" 2>/dev/null | head -1 | awk '{print $2}' | tr -d ';')
[ -z "$R" ] && { echo "no docroot for $D (vhost $V)"; exit 1; }
O=$(stat -c %U:%G "$R")
case $M in
put)
  cat > $R/lab-ini.php <<'P'
<?php
$before = ['memory_limit' => ini_get('memory_limit'), 'max_execution_time' => ini_get('max_execution_time'), 'upload_max_filesize' => ini_get('upload_max_filesize'), 'post_max_size' => ini_get('post_max_size'), 'max_input_vars' => ini_get('max_input_vars')];
$r1 = ini_set('memory_limit', '2048M'); $r2 = ini_set('max_execution_time', '3000'); @set_time_limit(5000);
$after = ['memory_limit' => ini_get('memory_limit'), 'max_execution_time' => ini_get('max_execution_time')];
echo json_encode(['php' => PHP_VERSION, 'before' => $before, 'ini_set_returned' => [$r1, $r2], 'after' => $after, 'user_ini' => ini_get('user_ini.filename')]), "\n";
P
  cat > $R/lab-sleep.php <<'P'
<?php
$s = (int)($_GET['s'] ?? 1); $t0 = time(); while (time() - $t0 < $s) { usleep(200000); }
echo "slept ", time() - $t0, " s, max_execution_time ", ini_get('max_execution_time'), "\n";
P
  printf 'memory_limit = 1536M\nmax_execution_time = 2000\nupload_max_filesize = 2000M\n' > $R/.user.ini
  chown $O $R/lab-ini.php $R/lab-sleep.php $R/.user.ini
  echo "files in $R (owner $O)"; ls -la $R | grep -E "lab-|user.ini"
  ;;
ini) curl -sS -m 20 -H "Host: $D" http://127.0.0.1/lab-ini.php; echo ;;
sleep) T0=$(date +%s); curl -sS -m 400 -o /tmp/sl.out -w "http %{http_code} after %{time_total}s\n" -H "Host: $D" "http://127.0.0.1/lab-sleep.php?s=$3"; cat /tmp/sl.out | head -3 ;;
pool)
  SK=$(grep -ohE 'unix:[^; ]+' $V /etc/nginx/celikpanel-managed.d/$D/*.conf 2>/dev/null | head -1 | sed 's/^unix://'); echo "socket $SK"
  P=$(grep -rlF "$SK" /etc/php/*/fpm/pool.d/ /etc/php/php-fpm.d/ 2>/dev/null | head -3)
  echo "pool file(s): $P"; for p in $P; do grep -nE "php_admin_value|php_value|php_flag|pm\.max_children|request_terminate|CelikPanel|celikpanel" $p | cut -c1-200; sha256sum $p | cut -c1-16; done
  ls /etc/nginx/celikpanel-managed.d/$D/ 2>/dev/null && cat /etc/nginx/celikpanel-managed.d/$D/site-settings.conf 2>/dev/null
  ;;
cleanup) rm -f $R/lab-ini.php $R/lab-sleep.php $R/.user.ini; echo cleaned ;;
esac
