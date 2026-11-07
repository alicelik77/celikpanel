export CELL=r7 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
grun ${N:-backup} g_jr.sh "$1" "$2" "$3" ${4:-40}
