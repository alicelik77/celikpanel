A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit
sleep 45
CELL=p124 MIP=192.0.3.190 BIP=192.0.3.191 Z=p124.test MOS=debian13 BOS=debian13 MVER=v0.1.0-alpha.124 BVER=v0.1.0-alpha.124 MKIND=pair-main BKIND=pair-backup-web MMEM=4096 BMEM=3072 bash $A/pairX.sh > $A/pairP.out 2>&1
