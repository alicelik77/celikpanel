# alpha.129: WordPress password, flush window sign-in, then Ubuntu 22.04 fresh 129 + forced failure with the rollback journal. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
bash $A/k3wp.sh > /dev/null 2>&1
bash $A/k4flush2.sh > /dev/null 2>&1
echo "== p129i (wp, flush) done $(date -u +%T)"
