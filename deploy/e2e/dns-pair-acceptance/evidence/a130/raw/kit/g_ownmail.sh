# guest (root): the owner's own edits before the removal (lab): a comment in CelikPanel's Dovecot quota drop-in and an
# owner systemd drop-in for Postfix. Lab only.
echo "# owner: keep this note" >> /etc/dovecot/conf.d/90-celikpanel-quota.conf; tail -1 /etc/dovecot/conf.d/90-celikpanel-quota.conf
mkdir -p /etc/systemd/system/postfix.service.d; printf '[Service]\n# owner drop-in (lab)\nNice=1\n' > /etc/systemd/system/postfix.service.d/owner.conf; systemctl daemon-reload; ls -la /etc/systemd/system/postfix.service.d
