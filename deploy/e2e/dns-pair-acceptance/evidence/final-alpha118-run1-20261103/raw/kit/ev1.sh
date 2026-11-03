source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
mkdir -p $B/ev
adm GET /api/v1/setup/operation --label op-stuck > $B/ev/a118-setup-operation-stuck-dns.json
gq g_ev1.sh f524d64a9c82ea767d1769184fc32515 > $B/ev/a118-guest-stuck-dns.txt 2>&1
cp $LOGS/single-a118.log $B/ev/a118-setup-run1.log
wc -l $B/ev/*
