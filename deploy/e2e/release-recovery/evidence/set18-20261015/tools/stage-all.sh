#!/bin/bash
set -u
echo set18-20261015 > /var/tmp/cp-set18/evidence-name.txt
S=<redacted>
bash $S/stage-lab.sh a s18-a debian13 debian13/fresh-a-lab-network-failed h18a
bash $S/stage-lab.sh b s18-b debian13 debian13/fresh-setup-b h18b
bash $S/stage-common.sh
