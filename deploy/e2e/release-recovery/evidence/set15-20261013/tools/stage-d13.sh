#!/bin/bash
echo set15-20261013 > /var/tmp/cp-set15-run/evidence-name.txt
J=<redacted>
bash $J/stage-lab.sh d13 s15-d13-a debian13 debian13/fresh-a h15d13
bash $J/stage-lab.sh ub s15-ub-a ubuntu ubuntu/fresh-a h15ub
bash $J/stage-lab.sh ugood s15-u83-good-a debian13 debian13/upd83-good-a h15ugood
bash $J/stage-lab.sh ugoodb s15-u83-good-b debian13 debian13/upd83-good-b h15ugoodb
