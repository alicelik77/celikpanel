# guest (root, read only): where is the EICAR test file now. Lab only.
echo "## $(date -u +%T) eicar files"
find /var/www /home /srv /var/lib -xdev -name '*eicar-test*' 2>/dev/null | while read f; do ls -la "$f"; done
find / -xdev -type d -iname '*quarantine*' -not -path '/proc/*' 2>/dev/null | head -5
