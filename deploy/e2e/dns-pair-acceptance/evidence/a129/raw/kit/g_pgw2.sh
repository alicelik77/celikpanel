# guest (root, read-only). Lab only.
journalctl -u celikpanel-agent --since "19:45" --no-pager -o cat | grep -iE "certificate|remote|readable" | tail -8 | cut -c1-300
cat /opt/celikpanel/release.commit; ls -la /usr/local/bin/celikpanel-agent /opt/celikpanel/bin 2>&1 | head -4
systemctl show celikpanel-agent -p ReadWritePaths -p ProtectSystem -p UMask | cut -c1-400
