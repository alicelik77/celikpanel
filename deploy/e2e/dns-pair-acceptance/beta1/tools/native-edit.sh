# Owner's native change on the main BIND (panel and agent stopped): add a TXT, bump the SOA serial, rndc reload.
set -e
f=$(rndc zonestatus beta1a.test | awk '/^files:/{print $2}')
echo "zone file: $f"
cp -a "$f" /root/beta1a-zone-before-owner-edit.zone
old=$(rndc zonestatus beta1a.test | awk '/^serial:/{print $2}'); new=$((old+1))
python3 - "$f" "$old" "$new" <<'PY'
import sys, re
p, old, new = sys.argv[1:]
s = open(p).read()
s2 = s.replace(old, new, 1)
assert s2 != s
s2 += 'owner-native 300 IN TXT "set by the owner with rndc while CelikPanel is stopped"\n'
open(p, "w").write(s2)
PY
named-checkzone beta1a.test "$f"
rndc reload beta1a.test
sleep 3
rndc zonestatus beta1a.test | grep serial
