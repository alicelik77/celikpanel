# guest (root): web settings checks for one site. usage: g_web.sh DOMAIN ID MODE. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
D=$1; ID=$2; M=$3
R=$(ls -d /var/www/celikpanel/subscriptions/*/sites/$ID/public_html 2>/dev/null | head -1)
O=$(stat -c %U:%G "$R" 2>/dev/null)
h() { curl -sSkL -m 20 -D - -o /tmp/body.out --resolve $D:80:127.0.0.1 --resolve $D:443:127.0.0.1 "http://$D$1" | grep -iE "^(HTTP|x-lab|expires|cache-control|content-type|x-celik|x-cache|x-page)" | tr -d '\r' | tr '\n' ' '; echo; head -c 200 /tmp/body.out; echo; }
case $M in
files)
  mkdir -p $R/errors; echo "<html><body>LAB CUSTOM 404 a128</body></html>" > $R/errors/404.html
  echo "<html><body>lab home.html first in the index</body></html>" > $R/home.html
  echo "lab mime" > $R/file.lab
  echo '<?php header("X-Lab-Php: 1"); echo "time=", microtime(true), "\n";' > $R/clock.php
  chown -R $O $R/errors $R/home.html $R/file.lab $R/clock.php; echo "docroot $R"; ls $R ;;
check)
  echo "-- /  (index: home.html first)"; h /
  echo "-- /file.lab (MIME)"; h /file.lab
  echo "-- /missing (custom 404)"; h /missing-page-a128
  echo "-- /clock.php twice (page cache)"; h /clock.php; sleep 1; h /clock.php ;;
conf)
  ls -la /etc/nginx/celikpanel-managed.d/$D/ 2>&1; for f in /etc/nginx/celikpanel-managed.d/$D/*.conf; do echo "## $f"; cat $f; done
  nginx -t 2>&1 | tail -1 ;;
hash) find /etc/nginx -type f | sort | xargs sha256sum | sha256sum | cut -c1-16 ;;
esac
