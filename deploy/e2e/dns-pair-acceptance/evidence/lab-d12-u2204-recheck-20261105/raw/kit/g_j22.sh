# guest (root, read-only): journal around the 01-dns failure. Lab only.
systemctl list-units 'celikpanel*' --no-legend --no-pager
journalctl --since "2026-11-04 20:43:05" --until "2026-11-04 20:43:35" --no-pager -o short-precise | grep -vE 'sshd|pam_unix|systemd-logind|session-|Started Session|sudo' | cut -c1-600 | head -120
