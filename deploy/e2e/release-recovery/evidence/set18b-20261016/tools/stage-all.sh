#!/bin/bash
set -u
source <redacted>
cat $R/evidence-name.txt
bash $J/stage-lab.sh b s18b-a debian13 debian13/fresh-setup-a h18ba
bash $J/stage-common.sh
