#!/bin/bash
# gq.sh CELL NODE 'command' : read-only diagnostics on a lab guest
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
b1 sh $1 $2 "$3" --label diag --limit ${4:-20000} | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("stdout","")); print(d.get("stderr","")[-500:])'
