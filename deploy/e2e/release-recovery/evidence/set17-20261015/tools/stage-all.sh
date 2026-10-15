#!/bin/bash
set -u
echo set17-20261015 > /var/tmp/cp-set17/evidence-name.txt
S=<redacted>
bash $S/stage-lab.sh d13 s17-d13-a debian13 debian13/fresh-a h17d13
bash $S/stage-lab.sh ugood s17-u85-good-a debian13 debian13/upd85-good-a h17ugood
bash $S/stage-lab.sh u86 s17-u86-good-a debian13 debian13/upd86-good-a h17u86
bash $S/stage-lab.sh u86b s17-u86-good-b debian13 debian13/upd86-good-b h17u86b
bash $S/stage-lab.sh udef s17-u85-def-a debian13 debian13/upd85-defective-a h17udef
bash $S/stage-lab.sh ubgood s17-ub-u85-good-a ubuntu ubuntu/upd85-good-a h17ubgood
bash $S/stage-lab.sh ubdef s17-ub-u85-def-a ubuntu ubuntu/upd85-defective-a h17ubdef
