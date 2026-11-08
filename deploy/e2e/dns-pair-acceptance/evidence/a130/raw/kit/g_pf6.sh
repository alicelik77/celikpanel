/usr/sbin/postfwd2 --dumpcache 2>/dev/null | grep -iE "rate|box1" | head -15
grep -nE "delrate|sub delete_rate|\"delrate" /usr/sbin/postfwd2 | head -8
