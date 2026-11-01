#!/bin/bash
# Network lab portal for the update topologies (C, D): a lab CA + a celikpanel.net leaf, the combined
# site (alpha.113 get.sh at the root, both signed releases, latest.txt as given) served by labnet on
# 192.0.2.61:443. usage: portalnet.sh up LATEST | latest VERSION | trust CELL NODE | down. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
N=/var/tmp/a114r2/portalnet
case "$1" in
up)
  mkdir -p $N && cd $N
  if [ ! -f ca.pem ]; then
    openssl req -x509 -newkey rsa:2048 -nodes -days 5 -subj "/CN=CelikPanel alpha.114 lab portal CA" -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign -keyout ca-key.pem -out ca.pem 2>/dev/null
    openssl req -new -newkey rsa:2048 -nodes -subj /CN=celikpanel.net -keyout tls-key.pem -out tls.csr 2>/dev/null
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:celikpanel.net\n' > tls.ext
    openssl x509 -req -in tls.csr -CA ca.pem -CAkey ca-key.pem -set_serial 1 -days 5 -extfile tls.ext -out tls.pem 2>/dev/null
    chmod 600 ca-key.pem tls-key.pem
  fi
  rm -rf site; mkdir -p site/releases
  cp -a /var/tmp/a114r2/portal-v0.1.0-alpha.113/site/. site/
  cp -a /var/tmp/a114r2/portal-v0.1.0-alpha.114/site/releases/v0.1.0-alpha.114 site/releases/
  echo "$2" > site/releases/latest.txt
  find site -type f | sort
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'ip addr add 192.0.2.61/24 dev lab0 2>/dev/null; rm -rf /root/a114-portal; mkdir -p /root/a114-portal; chmod 700 /root/a114-portal'"
  tar -C $N -cf - site tls.pem tls-key.pem | lab ssh beta1-lab labnet -- "sudo -n tar -xf - -C /root/a114-portal"
  lab ssh beta1-lab labnet -- "sudo -n tee /root/a114-portal/serve.py >/dev/null" < $H/portal_serve.py
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a114-portal 2>/dev/null; systemd-run --unit a114-portal -p RuntimeMaxSec=28800 python3 -I /root/a114-portal/serve.py; sleep 1; systemctl is-active a114-portal; curl -sS --cacert /dev/null -k --resolve celikpanel.net:443:192.0.2.61 https://celikpanel.net/releases/latest.txt'"
  ;;
latest)
  echo "$2" > $N/site/releases/latest.txt
  lab ssh beta1-lab labnet -- "echo $2 | sudo -n tee /root/a114-portal/site/releases/latest.txt"
  ;;
trust)
  # before anything of the product is on the guest: the lab CA in the native store, the name to labnet
  CELL=$2; NODE=$3
  lab ssh $CELL $NODE -- "sudo -n tee /usr/local/share/ca-certificates/a114-lab-portal.crt >/dev/null" < $N/ca.pem
  lab ssh $CELL $NODE -- "sudo -n sh -c 'update-ca-certificates >/dev/null 2>&1; echo \"192.0.2.61 celikpanel.net # alpha.114 lab portal\" >> /etc/hosts; curl -sS https://celikpanel.net/releases/latest.txt'"
  ;;
down)
  lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a114-portal 2>/dev/null; cat /root/a114-portal/requests.log 2>/dev/null | tail -40; rm -rf /root/a114-portal; ip addr del 192.0.2.61/24 dev lab0 2>/dev/null; echo down'"
  ;;
esac
