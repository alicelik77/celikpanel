P=$(pgrep -f "name beta1-r9-backup"); echo "kill $P"; [ -n "$P" ] && kill $P; sleep 3; pgrep -af qemu-system | grep -o "name [^ ]*"
tail -5 /var/tmp/a130o/logs/setup127-u22.log | cut -c1-200; pgrep -af setupV | grep -v pgrep | wc -l
