# alpha.128 round, fix check: the pair from 128 to the lab 129 label of fix/a128-lab (the round's fixes), Update pressed
# once each; then the native-file differences on the main. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C10upd-$CELL.log) 2>&1
echo "== portal latest = 129 (fix label 2) $(ts)"; bash $A/portalF.sh latest v0.1.0-alpha.130
for n in main; do grun $n g_snap.sh pre130 > /dev/null; done
CELL=r8 bash $A/upd.sh main
for n in main; do echo "== $n after $(ts)"; grun $n g_snap.sh post130 pre130; gshn $n 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"'; done
echo "== c10upd done $(ts)"
