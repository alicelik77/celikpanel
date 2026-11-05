# guest (root, read-only): the panel's journal around the lab DB hold. Lab only.
journalctl -u celikpanel-panel --since "04:46:40" --until "04:48:40" -o short-precise --no-pager | grep -viE 'GET /api/v1/setup/operation' | grep -iE 'busy|locked|wait|setup|step|retry|restart|start|stop|error' | cut -c1-400 | head -60
echo "-- restarts"; journalctl -u celikpanel-panel --since "04:44" -o short-precise --no-pager | grep -E 'Started|Stopped|Stopping' | head
