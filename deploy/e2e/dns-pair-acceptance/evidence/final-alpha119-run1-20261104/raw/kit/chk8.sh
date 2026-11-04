source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
cat > $A/g_nft.sh <<'G'
nft list tables; nft list table inet celikpanel_egress 2>&1 | head -40; ls -la /usr/lib/celikpanel-sandbox/ 2>&1; systemctl list-units 'celikpanel-sandbox*' 'celikpanel-egress*' --all --no-legend --plain 2>&1 | head
G
gq g_nft.sh
