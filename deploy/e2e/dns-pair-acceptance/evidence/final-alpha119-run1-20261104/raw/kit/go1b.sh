source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
bash $A/build.sh refs/tags/v0.1.0-alpha.117 v0.1.0-alpha.117 > $B/out-build117.out 2>&1; echo "build117 $(ts)"; cat $B/out-build117.out
PORTAL_VERSION=v0.1.0-alpha.118 PORTAL_SEQUENCE=118 bash $A/portal.sh $C118 > $B/portal118.out 2>&1; echo "portal118 exit $?"; cat $B/portal118.out | head -30
bash $A/portal.sh $C117 > $B/portal117.out 2>&1; echo "portal117 exit $?"; tail -3 $B/portal117.out
