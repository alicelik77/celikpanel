source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
gq g_tmp.sh > $B/ev/a118-guest-audit-dns.txt 2>&1
adm GET /api/v1/setup/operation --label op-stuck > $B/ev/a118-setup-operation-stuck-dns-2.json
cp $LOGS/single-a118.log $B/ev/a118-setup-run1.log
mv $LOGS/single-a118.log $LOGS/single-a118-try1.log
