source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/remotes/origin/release/alpha120 v0.1.0-alpha.120 > $B/out-build120.out 2>&1; bash $A/build.sh refs/tags/v0.1.0-alpha.119 v0.1.0-alpha.119 > $B/out-build119.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "builds done $(ts)"; grep -E "EXIT|\"commit\"" $B/out-build120.out $B/out-build119.out
bash $A/portal.sh $C118 > $B/portal120.out 2>&1; echo "portal120 exit $?"; grep -E "^(version|commit)=" $B/portal120.out
bash $A/portal.sh $C117 > $B/portal119.out 2>&1; echo "portal119 exit $?"; grep -E "^(version|commit)=" $B/portal119.out
echo "phase1 done $(ts)"
