# keep the Debian 12 update-failure evidence before the guest is replaced. Lab only (read-only).
export CELL=d5
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > $LOGS/C2-d5-debian12-update-failure.log 2>&1
echo "== Debian 12 (systemd 252), update 125 (bf0a5b9ee) -> lab 126 (64c22f5de): failure and blocked recovery"
grun main g_certdiag.sh; grun main g_isen.sh
gshn main "journalctl --since '2026-11-06 00:25:30' --until '2026-11-06 00:27:20' -o short-precise --no-pager -u celikpanel-panel -u celikpanel-agent -u 'celikpanel-self-update-*' -u celikpanel-release-recovery | grep -vE 'GET /api|SIGKILL' | cut -c1-400"
gshn main 'systemctl is-active celikpanel-panel celikpanel-agent; systemctl --failed --no-legend --plain | cat'
