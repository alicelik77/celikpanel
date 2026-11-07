tail -3 /var/tmp/a129o/logs/setup127-d12.log 2>/dev/null | cut -c1-200; pgrep -af qemu-system | grep -o "name [^ ]*"
