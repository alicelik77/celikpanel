# guest (root, read-only): the recovery runner's rollback lines (kill-who, services). Lab only.
journalctl --since "20:16:40" --until "20:17:20" -o short-iso --no-pager | grep -iE "kill|rollback\.sh|stopp|start|services|Geri alma|release-recovery" | cut -c1-260 | head -30
ps -eo args | grep -c "[r]ollback" ; grep -rl -- "--kill-who" /opt/celikpanel 2>/dev/null | head -3
