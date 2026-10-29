#!/bin/bash
# usage: infra.sh SUBCOMMAND ARGS...   (provider-zone ZONE /mnt/c/...json uploads the file first)
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
L=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/wtvm/deploy/e2e/dns-pair-acceptance/beta1
ssh_lab() { python3 -I $L/lab.py ssh beta1-lab labnet -- "$@"; }
ssh_lab "sudo -n tee /root/beta1/infra_guest.py >/dev/null" < $L/infra_guest.py
if [[ $1 == provider-zone ]]; then
  ssh_lab "sudo -n tee /root/beta1/upload-$2.json >/dev/null" < "$3"
  ssh_lab "sudo -n python3 -I /root/beta1/infra_guest.py provider-zone $2 /root/beta1/upload-$2.json"
else
  ssh_lab "sudo -n python3 -I /root/beta1/infra_guest.py $*"
fi
