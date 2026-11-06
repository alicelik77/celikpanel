journalctl -u celikpanel-release-recovery.service --since 13:29:20 --until 13:30:00 -o short-iso --no-pager | grep -v "Created symlink" | cut -c1-260
