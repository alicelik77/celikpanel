# guest (root, read only): what is left in Drupal's document root after removal. Lab only.
f=$(grep -l "drup.apps118.test" /etc/nginx/sites-enabled/* | head -1); r=$(grep -m1 ' root ' $f | awk '{print $2}' | tr -d ';'); echo "root $r"; ls -la $r
