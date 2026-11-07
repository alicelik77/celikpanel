# common header of the per-system case scripts (sourced). env: S IPU. Lab only.
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id 2>/dev/null); PID=$(cat $T/planBasic.id 2>/dev/null)
