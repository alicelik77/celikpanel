#!/bin/bash
d=$(ls -d /var/tmp/cp-release-drill-$1/evidence/*/upd1/*/ | tail -1)
python3 -I -c '
import json,sys
d=json.load(open(sys.argv[1]))
for k in ("route_get_1_1_1_1","block_check_203_0_113_1_443","pebble_root","directory","pebble_version","apt_sources"):
    print(k, "=", str(d.get(k))[:300])
print("pkg", d["packages_install"]["install_rc"], d["packages_install"]["nftables_service"])
print("dns", [(c["resolver"],c["proto"],c["name"],c.get("rcode"),c.get("answers"),c.get("error")) for c in d["dns_checks"]])
print("allowed", d["tables"]["allowed_hosts"], d["tables"]["allowed_v4"], d["tables"]["allowed_v6"])
' $d/steps/04-set18-lab-network/set18-lab-network.json
