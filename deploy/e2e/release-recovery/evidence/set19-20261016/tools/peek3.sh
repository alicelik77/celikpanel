#!/bin/bash
# set19: read-only: the renewal readings of one lab. usage: peek3.sh LABNAME
E=$(find /var/tmp/cp-release-drill-$1/evidence -maxdepth 4 -type d -name 'upd1-debian13-good-*' | head -1)
D=$(ls -d $E/steps/*set19-certbot-readings)
cat $D/set19-certbot-readings.json
echo; echo "=== units after"; cat $D/certbot-units-after.txt
echo "=== journals"; cat $D/journals-and-observations.txt | head -150
