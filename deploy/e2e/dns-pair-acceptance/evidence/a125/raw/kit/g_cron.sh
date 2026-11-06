# guest (root): show the crontab of site users; OWNER USER MAILTO sets an owner MAILTO line by hand (lab load standing for the owner).
case "$1" in
show) shift; for u in "$@"; do echo "--- crontab -u $u"; crontab -u $u -l 2>&1 | sed 's/^/    /'; done; echo "--- root crontab MAILTO lines: $(crontab -l 2>/dev/null | grep -c MAILTO)";;
owner) (echo "MAILTO=\"$3\""; crontab -u $2 -l 2>/dev/null) | crontab -u $2 - && echo "owner set MAILTO for $2";;
esac
