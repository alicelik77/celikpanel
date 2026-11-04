u=celikrtbuild-1.service
systemctl show $u -p Requires,After,BindsTo,Wants,ActiveState,SubState,Result,LoadState,InvocationID 2>&1 | cut -c1-300
b=celikpanel-sandbox@celikrtbuild-1.service; echo "## $b: $(systemctl is-active $b) $(systemctl show $b -p Result --value)"
journalctl -u $b --no-pager -o short-precise 2>/dev/null | tail -12 | cut -c1-300
journalctl --no-pager -o short-precise --since "02:56:45" --until "02:56:57" 2>/dev/null | grep -iE "celikrtbuild|celikpanel-sandbox|dependency|job" | head -12 | cut -c1-300
