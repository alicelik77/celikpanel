# guest (root, read-only): the start-up mail work line on a server without mail, per panel start. Lab only.
journalctl -u celikpanel-panel -o short-iso --no-pager | grep -E "Starting CelikPanel Backend|startup mail work" | cut -c1-330
