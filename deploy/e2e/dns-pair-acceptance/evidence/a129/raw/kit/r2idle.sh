export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/R2b-$CELL.log) 2>&1
S0=$(cat $T/cust2-idle-start); echo "== B.5' idle timeout (15 min): cust2's session unused for $(( ($(date +%s) - S0) / 60 )) min $(ts)"
python3 -I $A/raw.py $L $CELL main GET /api/v1/auth/sessions --as $T/cred-cust2.json --show 220
