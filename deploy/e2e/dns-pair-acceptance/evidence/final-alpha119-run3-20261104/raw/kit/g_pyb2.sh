journalctl --no-pager -o short-precise --since "01:50:33" --until "01:50:35" 2>/dev/null | grep -vE "sshd|pam_unix|sudo|session" | head -30 | cut -c1-350
