ls -d /var/tmp/a128r1* | cat -A
rm -rf "/var/tmp/a128r1"$'\r'
ls -d /var/tmp/a128r1* 2>&1 | cat -A
pgrep -af "build.sh|build-dist" | head
