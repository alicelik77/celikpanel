source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
BUMP=1 bash $A/build.sh refs/heads/release/alpha118 v0.1.0-alpha.118 > $B/out-build118.out 2>&1; cat $B/out-build118.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build118.out); echo "C118L=$C"
bash $A/portal.sh $C > $B/portal118.out 2>&1; echo "portal118 exit $?"; head -12 $B/portal118.out
