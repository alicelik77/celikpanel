cd /var/tmp/a114r4/shots
for f in snew4-*.txt; do echo "== $f: join mentions=$(grep -ciE 'will join|join|katılacak|katilacak|katıl' $f)"; grep -iE 'join|katıl|mail\.' $f | head -6 | cut -c1-240; done
