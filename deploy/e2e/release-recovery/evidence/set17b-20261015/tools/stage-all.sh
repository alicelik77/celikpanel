#!/bin/bash
set -u
echo set17b-20261015 > /var/tmp/cp-set17b/evidence-name.txt
S=<redacted>
bash $S/stage-lab.sh u86a s17b-u86-a debian13 debian13/upd86-good-a0-not-held h17bu86a
bash $S/stage-lab.sh u86a2 s17b-u86-a2 debian13 debian13/upd86-good-a h17bu86a2
bash $S/stage-lab.sh u86b s17b-u86-b debian13 debian13/upd86-good-b h17bu86b
bash $S/stage-lab.sh ctl s17b-u85-ctl debian13 debian13/upd85-control-a h17bctl
bash $S/stage-common.sh
