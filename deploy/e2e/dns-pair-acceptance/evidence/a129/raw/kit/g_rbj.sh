# guest (root, read-only): the rollback evidence since $1 (HH:MM:SS). Lab only.
echo '--- systemctl --failed'; systemctl list-units --failed --all --no-legend --plain | cat; echo '(failed end)'
journalctl --since "$1" -o short-iso --no-pager | grep -iE 'rollback|rolled back|restor|recovery_required|release recovery|CELIKPANEL_UPDATE|Recovery dispatch|Verified snapshot|frozen|freeze|kill-who|stopped before|Starting the stopped' | cut -c1-260 | tail -40
echo "--- release $(cat /opt/celikpanel/release.commit 2>/dev/null) ; panel $(systemctl is-active celikpanel-panel) agent $(systemctl is-active celikpanel-agent)"
