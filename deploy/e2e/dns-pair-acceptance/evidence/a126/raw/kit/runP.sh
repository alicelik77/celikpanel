# alpha.126 round: the owner's pair at the published alpha.125 (lab copy): Debian 13 PowerDNS main + mail,
# Ubuntu 24.04 BIND backup joined by code, both through get.sh. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit
dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failure in dmesg"; exit 2; }
CELL=p6 MIP=192.0.3.190 BIP=192.0.3.191 Z=p6.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.125 BVER=v0.1.0-alpha.125 MKIND=pair-main BKIND=pair-backup-web BENGINE=bind MMEM=3584 BMEM=2304 TRUST=1 bash $A/pairX.sh > $A/pairP.out 2>&1
source $A/env.sh
for n in main backup; do opnow p6 $n | head -1 | cut -c1-300; done
free -m | head -2
echo "runP done $(date -u +%T)"
