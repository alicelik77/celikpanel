source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
for c in u22 d12; do CELL=$c; echo "==== $c"; GQT=60 gq g_bindbytes.sh; done 2>&1 | tee $LOGS/bind-vendor-bytes-u22-d12.txt
