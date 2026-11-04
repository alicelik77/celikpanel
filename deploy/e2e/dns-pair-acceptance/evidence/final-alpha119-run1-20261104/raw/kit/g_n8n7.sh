# guest (root): n8n unit sandbox generation 3 view; SMTP egress check from inside the unit's cgroup. Lab only.
u=${1:-celiksvc-1}.service
echo "## $(date -u +%T) $u active=$(systemctl is-active $u)"
systemctl cat $u 2>/dev/null | grep -E "^(EnvironmentFile|BindPaths|BindReadOnlyPaths|Requires|BindsTo|After|PartOf|X-CelikPanel|# .*generation|NoExecPaths|RestrictAddressFamilies|SocketBind|IPAddressDeny)" | head -20
grep -iE "generation" /etc/systemd/system/$u | head -3
echo "## bind unit: celikpanel-sandbox@${u%.service}.service active=$(systemctl is-active celikpanel-sandbox@${u%.service}.service)"; ls -la /run/celikpanel-sandbox/ 2>&1 | head
stat -c '%n %U:%G %a' /etc/celikpanel-svcapps /etc/celikpanel-svcapps/* 2>&1
echo "## egress table"; nft list set inet celikpanel_egress smtp_blocked 2>&1 | head -12; nft list table inet celikpanel_egress 2>/dev/null | grep -iE "counter|smtp" | head -6
CG=/sys/fs/cgroup$(systemctl show $u -p ControlGroup --value); echo "## cgroup $CG"
echo "## SMTP attempts from a helper process placed in the unit's cgroup (lab check)"
bash -c "echo \$\$ > $CG/cgroup.procs && exec python3 - <<'P'
import socket
for host in ('127.0.0.1','192.0.2.25','1.1.1.1'):
    for port in (25,465,587):
        s=socket.socket(); s.settimeout(4)
        try: s.connect((host,port)); r='CONNECTED (escape)'
        except Exception as e: r='refused: '+type(e).__name__+' '+str(e)[:60]
        print('  smtp', host, port, r); s.close()
P" 2>&1
nft list table inet celikpanel_egress 2>/dev/null | grep -iE "counter" | head -6
