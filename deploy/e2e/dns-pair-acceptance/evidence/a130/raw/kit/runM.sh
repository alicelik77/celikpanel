# D4 mail round on Debian 12 with the lab 129 label of fix/a129-lab (fresh install, setup, mail limits). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
mkdir -p $B $LOGS; bash $A/sync.sh
python3 -I $L/lab.py net-up > /dev/null; python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
( timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up $? $(ts)" ) &
BUMP=129 PREVC=$C128 bash $A/build.sh refs/remotes/origin/fix/a129-lab v0.1.0-alpha.129 > $B/out-build129.out 2>&1
wait
grep -E "^commit|^lab bump|EXIT|FAILED|\"sha256\"" $B/out-build129.out
C9=$(sed -n "s/^lab bump commit //p" $B/out-build129.out | head -1); echo $C9 > $B/c129
bash $A/portal.sh $C9 > $B/portal129.out 2>&1; echo "portal129 exit $?"
PREVV=v0.1.0-alpha.129 NEXTV=v0.1.0-alpha.129 bash $A/portalF.sh up v0.1.0-alpha.129 > $B/portalnet-up.out 2>&1; tail -1 $B/portalnet-up.out
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q sshpass lftp swaks >/dev/null 2>&1; echo labnet-tools-ok"
CELLX=m12 S=d12 OS=debian12 IPU=192.0.3.231 VER=v0.1.0-alpha.129 bash $A/setupV.sh > /dev/null 2>&1; tail -8 $LOGS/setup127-d12.log
CELLX=m12 S=d12 IPU=192.0.3.231 bash $A/k4mail.sh > /dev/null 2>&1; sed -n "/4.1 postfwd/,\$p" $LOGS/C4mail-d12.log | grep -v "235 2.7.0" | grep -vE "^#|^\[Service|^ExecStart" | cut -c1-260
