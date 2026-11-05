A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit
until grep -q "^done" $A/build3.out 2>/dev/null; do sleep 10; done; grep -q "portal124c exit 0" $A/build3.out || { echo "build3 failed"; exit 1; }
CELL=p2 MIP=192.0.3.190 BIP=192.0.3.191 Z=p2.test MOS=debian13 BOS=debian13 MVER=v0.1.0-alpha.124 BVER=v0.1.0-alpha.124 MKIND=pair-main-web BKIND=pair-backup-web MMEM=3584 BMEM=2560 bash $A/pairX.sh > $A/pairP2.out 2>&1
bash $A/R6r.sh > $A/R6r.out 2>&1
