# alpha.130 round: the first server's setup stopped at the mail profile (Roundcube download HTTP 504, the
# internet's answer); the owner presses Start again. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh; source $H/common.sh
exec > >(tee -a $LOGS/pair-s0-retry.log) 2>&1
echo "== press Start again on the first server $(ts)"


TRIES=11 EVERY=15 wait_status s0 main succeeded failed
echo "== retry done $(ts)"
