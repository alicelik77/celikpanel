#!/bin/bash
# usage (Windows side): write commands to cmd.sh, then: wsl ... bash run.sh  (runs cmd.sh with helpers defined)
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/b1
B="bash $H/b1.sh"
INFRA="bash $H/infra.sh"
W=$H/wiz
LAB="python3 -I $H/../wtvm/deploy/e2e/dns-pair-acceptance/beta1/lab.py"
echo "{}" > /tmp/empty.json
source $H/cmd.sh
