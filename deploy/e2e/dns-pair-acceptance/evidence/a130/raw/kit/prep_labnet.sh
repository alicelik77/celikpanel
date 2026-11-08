# alpha.130 round: labnet clients + SASL relay (lab only).
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib7.sh
pkill -f "kit/k2.sh" && echo "stopped an earlier k2 run"
exec > >(tee -a $LOGS/N0prep.log) 2>&1
mkcred relayuser
nup n_prep.sh >/dev/null
pw relayuser | timeout 600 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_prep.sh relayuser"
