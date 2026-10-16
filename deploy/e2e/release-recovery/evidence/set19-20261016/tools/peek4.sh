#!/bin/bash
# set19: read-only: one lab's preparation record. usage: peek4.sh LABNAME
E=$(find /var/tmp/cp-release-drill-$1/evidence -maxdepth 4 -type d -name 'upd1-debian13-good-*' | head -1)
D=$(ls -d $E/steps/*set19-certbot-lab-preparation)
cat $D/set19-certbot-preparation.json; echo
awk '/== after/{f=1} f' $D/set19-certbot-preparation.txt | head -40
