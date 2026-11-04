date -u +%T; ps -eo pid,etime,cmd | grep -E "apt|dpkg|unattended" | grep -v grep | head
cat /var/lib/celikpanel-agent-private/service-mutations.json | python3 -c 'import json,sys; d=json.load(sys.stdin); print("active", d.get("active_request_id")); [print(" ", j["kind"], j["target"], j["status"], j.get("phase"), j.get("updated_at")) for j in sorted(d["jobs"].values(), key=lambda x:x.get("updated_at",""))[-5:]]'
journalctl -u celikpanel-panel -u celikpanel-agent --no-pager --since -6min 2>/dev/null | grep -iE "busy|tools|mutation|in progress|operation" | tail -8 | cut -c1-260
