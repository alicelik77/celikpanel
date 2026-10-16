#!/bin/bash
# set19: read-only look at one lab's driver evidence. usage: peek.sh LABNAME
root=/var/tmp/cp-release-drill-$1
find $root/evidence -maxdepth 4 -type d | head -20
E=$(find $root/evidence -maxdepth 4 -type d -name 'upd1-debian13-good-*' | head -1)
echo "E=$E"; ls $E; ls $E/steps 2>/dev/null | tail -14
