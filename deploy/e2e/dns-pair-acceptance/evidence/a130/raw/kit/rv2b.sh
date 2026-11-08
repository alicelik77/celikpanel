# alpha.130 re-verify on B: Postfix removal with an owner drop-in (retry after the busy answer). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2b-$CELL.log) 2>&1
N=backup; b1 login $CELL $N > /dev/null 2>&1
echo "== retry Postfix removal $(ts)"
for i in $(seq 1 12); do r=$(admn $N POST /api/v1/service/uninstall '{"service_id":"postfix"}' --label postfix-remove --timeout 900 | sbrief 1500); echo "$r" | grep -q HOST_MUTATION_BUSY || break; sleep 20; done; echo "$r"
gshn $N 'ls -la /etc/systemd/system/postfix.service.d/ 2>&1; dpkg -l postfix 2>/dev/null | grep -E "^(ii|rc)" | cut -c1-60; ls /etc/postfix 2>&1 | head -3'
echo "== rv2b done $(ts)"
