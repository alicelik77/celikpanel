#!/bin/bash
# Network lab portal for the update topologies (C, D): a lab CA + a celikpanel.net leaf, the combined
# site (alpha.113 get.sh at the root, both signed releases, latest.txt as given) served by labnet on
# 192.0.2.61:443. usage: portalnet.sh up LATEST | latest VERSION | trust CELL NODE | down. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
N=/var/tmp/a118r1/portalnet
case "$1" in
up)
  mkdir -p $N && cd $N
  if [ ! -f ca.pem ]; then
    openssl req -x509 -newkey rsa:2048 -nodes -days 5 -subj "/CN=CelikPanel alpha.118 lab portal CA" -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign -keyout ca-key.pem -out ca.pem 2>/dev/null
    openssl req -new -newkey rsa:2048 -nodes -subj /CN=celikpanel.net -keyout tls-key.pem -out tls.csr 2>/dev/null
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:celikpanel.net\n' > tls.ext
    openssl x509 -req -in tls.csr -CA ca.pem -CAkey ca-key.pem -set_serial 1 -days 5 -extfile tls.ext -out tls.pem 2>/dev/null
    chmod 600 ca-key.pem tls-key.pem
  fi
  rm -rf site; mkdir -p site/releases
  cp -a /var/tmp/a118r1/portal-${PREVV:-v0.1.0-alpha.117}/site/. site/
  cp -a /var/tmp/a118r1/portal-${NEXTV:-v0.1.0-alpha.118}/site/releases/${NEXTV:-v0.1.0-alpha.118} site/releases/
  echo "$2" > site/releases/latest.txt
  find site -type f | sort
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'ip addr add 192.0.2.61/24 dev lab0 2>/dev/null; rm -rf /root/a114-portal; mkdir -p /root/a114-portal; chmod 700 /root/a114-portal'"
  tar -C $N -cf - site tls.pem tls-key.pem | lab ssh beta1-lab labnet -- "sudo -n tar -xf - -C /root/a114-portal"
  lab ssh beta1-lab labnet -- "sudo -n tee /root/a114-portal/serve.py >/dev/null" < $H/portal_serve.py
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a114-portal 2>/dev/null; systemd-run --unit a114-portal -p RuntimeMaxSec=28800 python3 -I /root/a114-portal/serve.py; sleep 1; systemctl is-active a114-portal; curl -sS --cacert /dev/null -k --resolve celikpanel.net:443:192.0.2.61 https://celikpanel.net/releases/latest.txt'"
  ;;
add)
  # run 3: one more signed release (the lab 115 label of the same candidate) on the running portal
  cp -a /var/tmp/a114r6/portal-$2/site/releases/$2 $N/site/releases/
  tar -C $N -cf - site/releases/$2 | lab ssh beta1-lab labnet -- "sudo -n tar -xf - -C /root/a114-portal"
  lab ssh beta1-lab labnet -- "sudo -n find /root/a114-portal/site/releases -maxdepth 1"
  ;;
latest)
  echo "$2" > $N/site/releases/latest.txt
  lab ssh beta1-lab labnet -- "echo $2 | sudo -n tee /root/a114-portal/site/releases/latest.txt"
  ;;
trust)
  # before anything of the product is on the guest: the lab CA in the native store, the name to labnet
  CELL=$2; NODE=$3
  # run 3: Arch keeps its anchors in /etc/ca-certificates/trust-source/anchors (update-ca-trust)
  lab ssh $CELL $NODE -- "sudo -n sh -c 'if command -v update-ca-certificates >/dev/null; then cat > /usr/local/share/ca-certificates/a114-lab-portal.crt; update-ca-certificates >/dev/null 2>&1; else cat > /etc/ca-certificates/trust-source/anchors/a114-lab-portal.crt; update-ca-trust; fi'" < $N/ca.pem
  # idempotent; re-run after an install restart (Arch's cloud-init rewrites /etc/hosts at boot)
  lab ssh $CELL $NODE -- "sudo -n sh -c 'grep -q \"alpha.114 lab portal\" /etc/hosts || echo \"192.0.2.61 celikpanel.net # alpha.114 lab portal\" >> /etc/hosts; curl -sS https://celikpanel.net/releases/latest.txt'"
  ;;
down)
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a114-portal 2>/dev/null; cat /root/a114-portal/requests.log 2>/dev/null | tail -40; rm -rf /root/a114-portal; ip addr del 192.0.2.61/24 dev lab0 2>/dev/null; echo down'"
  ;;
esac
