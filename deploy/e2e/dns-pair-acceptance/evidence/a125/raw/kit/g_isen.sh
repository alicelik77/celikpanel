systemctl --version | head -1
out=$(systemctl is-enabled certbot-renew.timer 2>/dev/null); echo "rc=$? stdout=[$out]"
