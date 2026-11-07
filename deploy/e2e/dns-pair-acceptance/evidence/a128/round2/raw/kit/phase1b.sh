source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/env.sh
echo "start $(ts)"
bash $A/build.sh refs/tags/v0.1.0-alpha.127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
grep -E "^commit|EXIT|budget|FAILED|\"commit\"|\"version\"|\"sha256\"" $B/out-build127.out
bash $A/portal.sh $C127 > $B/portal127.out 2>&1; echo "portal127 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal127.out
PREVV=v0.1.0-alpha.127 NEXTV=v0.1.0-alpha.128 bash $A/portalF.sh up v0.1.0-alpha.127 > $A/portalnet-up.out 2>&1; tail -1 $A/portalnet-up.out
echo "phase1b done $(ts)"
