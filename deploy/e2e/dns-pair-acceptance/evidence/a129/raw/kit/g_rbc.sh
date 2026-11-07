journalctl -u celikpanel-release-recovery.service --since "$1" -o cat --no-pager | grep -ciE "rollback complete|returned to their state|stopped before|rolled back"
