#!/bin/bash
# Bring up the beta1 lab network and the lab guest (labnet), then prepare it. Lab only.
set -euo pipefail
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HERE=$(cd "$(dirname "$0")" && pwd)
mkdir -p /var/tmp/cp-beta1/keys /var/tmp/cp-beta1/lab
[[ -f /var/tmp/cp-beta1/keys/id_ed25519 ]] || ssh-keygen -q -t ed25519 -N '' -C cp-beta1 -f /var/tmp/cp-beta1/keys/id_ed25519
python3 -I "$HERE/lab.py" net-up
if [[ ! -d /var/tmp/cp-beta1/lab/cells/beta1-lab/labnet ]]; then python3 -I "$HERE/lab.py" infra-start; fi
python3 -I "$HERE/lab.py" wait-ssh beta1-lab labnet 600
python3 -I "$HERE/lab.py" ssh beta1-lab labnet -- "sudo -n install -d -m 0700 /root/beta1 && sudo -n tee /root/beta1/infra_guest.py >/dev/null" < "$HERE/infra_guest.py"
timeout 2400 python3 -I "$HERE/lab.py" ssh beta1-lab labnet -- "sudo -n python3 -I /root/beta1/infra_guest.py prepare" > /var/tmp/cp-beta1/lab/infra-prepare.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps({k: d[k] for k in ("apt_install","versions","pebble_root","self_checks","blocked_addresses")}, indent=1)[:5000])' /var/tmp/cp-beta1/lab/infra-prepare.json
python3 -I "$HERE/lab.py" ssh beta1-lab labnet -- "sudo -n cat /root/beta1/pebble-root.pem" > /var/tmp/cp-beta1/lab/pebble-root.pem
openssl x509 -noout -subject -fingerprint -sha256 -in /var/tmp/cp-beta1/lab/pebble-root.pem
echo INFRA-UP
