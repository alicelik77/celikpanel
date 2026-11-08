# guest (root, read-only): failed units, the release recovery and the panel stop/start times since $1. Lab only.
echo "--- systemctl --failed"; systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"
[ "$2" = after ] && exit 0
journalctl --since "$1" -o short-iso --no-pager | grep -E "(Stopped|Started) celikpanel-(panel|agent)\.service|Rollback complete|rollback refused|not idle|Recovery dispatch|Verified snapshot|recovery child failed" | cut -c1-200 | head -20
cat /opt/celikpanel/release.commit 2>/dev/null; cat /opt/celikpanel/RELEASE_COMMIT 2>/dev/null | head -1
