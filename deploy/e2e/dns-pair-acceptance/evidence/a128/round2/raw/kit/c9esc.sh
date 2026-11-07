# alpha.127 case 9: the site sandbox escape kit (deploy/e2e/sandbox-escape, candidate 4ee915bb8) in node mode on the
# Debian 13 main and the Ubuntu 24.04 joined server (lab VMs only). Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C9esc-$CELL.log) 2>&1
R=$B/repo-v0.1.0-alpha.127; KD=$B/esckit; rm -rf $KD; mkdir -p $KD
git -c safe.directory="*" -C $R archive rel | tar -x -C $KD --exclude=web
echo "== build the kit $(ts) (commit $(git -c safe.directory='*' -C $R rev-parse rel^{commit}))"
( cd $KD && GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache timeout 900 bash deploy/e2e/sandbox-escape/run.sh build 2>&1 | tail -5 )
ls -la $KD/deploy/e2e/sandbox-escape/bin
V=$(cat $T/nodev.main)
echo "-- Node.js $V on the joined server (API, as the Components page)"
admn backup GET /api/v1/runtimes/node --label nodes | sbrief 300
admn backup GET /api/v1/runtimes/node --label nodes | grep -q "$V" || { admn backup POST /api/v1/runtimes/node "{\"version\":\"$V\",\"request_id\":\"$(rid)\"}" --label node-install --timeout 900 | sbrief 400; for i in $(seq 1 60); do admn backup GET /api/v1/runtimes/node --label nodes | grep -q "$V" && break; sleep 10; done; }
for n in main backup; do
  echo "== escape kit, node mode, on $n $(ts)"
  tar -C $KD/deploy/e2e -cf - sandbox-escape | lab ssh $CELL $n -- "sudo -n sh -c 'rm -rf /root/sandbox-escape; tar -xf - -C /root'"
  timeout 1500 python3 -I $L/lab.py ssh $CELL $n -- "sudo -n env NODE_VERSION=$V bash /root/sandbox-escape/run.sh node" > $LOGS/C9esc-$n.txt 2>&1; echo "exit $?"
  grep -E "^ *[0-9]+ |PASS|FAIL|SKIP|level|LEVEL|summary|Summary" $LOGS/C9esc-$n.txt | tail -40
done
echo "== c9esc done $(ts)"
