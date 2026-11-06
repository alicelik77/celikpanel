# guest (root, read-only): a refused update attempt and its rollback; unit states in between. $1 since, $2 until. Lab only.
S=$1; U=$2
echo "--- release-recovery runner lines $S-$U"
journalctl -u celikpanel-release-recovery.service --since "$S" --until "$U" -o short-iso --no-pager | cut -c1-260 | grep -v "Created symlink" | grep -vE "Consumed|start request repeated" | tail -40
echo "--- systemd unit results $S-$U (celikpanel units: failed / finished / deactivated)"
journalctl --since "$S" --until "$U" -o short-iso --no-pager _PID=1 | grep -E "celikpanel" | grep -iE "fail|result|Deactivated|Finished|Started|Stopp" | cut -c1-220 | tail -40
echo "--- update failure and rollback lines"; journalctl --since "$S" --until "$U" -o cat --no-pager | grep -E "CELIKPANEL_UPDATE_FAILURE|Rollback complete|Geri alma|rolled back|restored commit|Recovery dispatch|Verified snapshot" | cut -c1-300 | tail -12
echo "--- panel answered (first panel start after the attempt)"; journalctl -u celikpanel-panel --since "$S" --until "$U" -o short-iso --no-pager | grep -E "Starting CelikPanel|listening|Started" | cut -c1-200 | tail -6
echo "--- systemctl --failed now"; systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"
echo "release.commit $(cat /opt/celikpanel/release.commit 2>/dev/null || ls /opt/celikpanel | head -3)"
