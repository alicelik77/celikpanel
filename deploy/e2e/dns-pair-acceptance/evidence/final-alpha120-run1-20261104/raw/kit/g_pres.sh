# guest (root, read only): PrestaShop document root after the failed setup step. Lab only.
f=$(grep -l "server_name.* pres.apps118.test" /etc/nginx/sites-enabled/* | head -1); r=$(grep -m1 ' root ' "$f" | awk '{print $2}' | tr -d ';'); echo "## root $r"
ls -la "$r" | head -40; ls -d "$r"/admin* 2>&1; stat -c '%n %U %a' "$r"/admin* 2>/dev/null
