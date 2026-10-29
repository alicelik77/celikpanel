echo "== resolver queries for denied names (by name, count)"; grep -E 'celikpanel|letsencrypt|lencr|pki.goog|zerossl|buypass|sectigo|digicert' /var/log/unbound/beta1.log | grep -oE '(info: [0-9.]+ )?[a-z0-9.-]*(celikpanel|letsencrypt|lencr|pki\.goog|zerossl|buypass|sectigo|digicert)[a-z.]*\. [A-Z]+' | sort | uniq -c | sort -rn | head -20
echo "== forward drops / blocked"; journalctl -k --no-pager | grep -cE 'beta1-fwd-drop'; journalctl -k --no-pager | grep -cE 'beta1-blocked'
journalctl -k --no-pager | grep -E 'beta1-(fwd-drop|blocked)' | grep -oE 'SRC=[0-9.]+ DST=[0-9.]+.*PROTO=[A-Z]+ SPT=[0-9]+ DPT=[0-9]+' | awk '{print $2, $NF}' | sort | uniq -c | sort -rn | head -15
echo "== nft counters"; nft list table ip beta1lab | grep counter
echo "== pebble issued"; journalctl -u beta1-pebble --no-pager | grep -c 'Issued certificate'
echo "== greeter connections"; wc -l < /root/beta1/greeter.jsonl 2>/dev/null
