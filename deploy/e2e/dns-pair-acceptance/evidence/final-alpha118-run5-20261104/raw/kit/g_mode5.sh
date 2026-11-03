# guest (root, read only): document root modes across sites (setgid bit) and backup content modes. Lab only.
for d in /var/www/celikpanel/subscriptions/*/sites/*/public_html; do stat -c '%n %U:%G %A %a' $d; done
ls -la --time-style=+%T /var/www/celikpanel/subscriptions/3/sites/2/public_html/
