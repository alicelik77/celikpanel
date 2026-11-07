# guest: where the Node site's environment lives (secret value must not be in a world-readable unit file). $1 = domain id
ID=$1
ls -la /etc/systemd/system/ | grep "celikapp-$ID" ; ls -la /etc/systemd/system/celikapp-$ID.service.d/ 2>/dev/null
echo "unit files naming LAB_SECRET: $(grep -rl LAB_SECRET /etc/systemd/system/celikapp-$ID* 2>/dev/null | tr '\n' ' ')"
echo "secret value in unit files: $(grep -rl s3cret-value /etc/systemd/system/celikapp-$ID* 2>/dev/null | wc -l); in systemctl show Environment: $(systemctl show celikapp-$ID -p Environment | grep -c s3cret-value)"
for f in $(grep -rl -e LAB_SECRET /etc/systemd/system/celikapp-$ID* /var/lib/celikpanel* /etc/celikpanel 2>/dev/null | head -5); do stat -c "%A %U:%G %n" $f; done
