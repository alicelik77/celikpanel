source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
lab ssh a4-c main -- "sudo -n sh -s" <<'R'
u=$(systemctl list-units --all --no-legend 'celikpanel-self-update-*' | awk '{print $1}' | tail -1); echo "unit: $u"
journalctl --no-pager -o short-iso --since '07:28:30' --until '07:40:00' -u "$u" | grep -E '^.{25}.*(==>|!!|die|start check|rollback|recover|refus|DNS|dns)' | cut -c1-300 | tail -40
echo ---- recovery
journalctl --no-pager -o short-iso --since '07:28:30' -u celikpanel-release-recovery.service | cut -c1-300 | tail -30
echo ---- status
/usr/libexec/celikpanel/recovery status --json 2>&1 | head -c 1500; echo
ls -la /var/lib/celikpanel/ 2>/dev/null | grep -i -E 'release|pending|marker' | head
R
