source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
GQT=60 gq gA0.sh 2 > $B/ev/a118-guest-final-quota24.txt 2>&1; grep -A30 "## dovecot" $B/ev/a118-guest-final-quota24.txt | head -45
cat > $A/g_fin.sh <<'G'
echo "## panics"; journalctl -u celikpanel-panel -u celikpanel-agent --no-pager 2>/dev/null | grep -ciE 'panic:|goroutine [0-9]+ \['
systemctl --failed --no-legend --no-pager | head
G
gq g_fin.sh
