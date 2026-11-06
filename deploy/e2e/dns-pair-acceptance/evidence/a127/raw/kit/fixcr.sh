ls -d /var/tmp/a127r1* | cat -A
rm -rf "/var/tmp/a127r1"$'\r'
ls -d /var/tmp/a127r1* 2>&1 | cat -A
pgrep -af qemu-system | grep -o "name [^ ]*"
