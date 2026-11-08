ls -la --time-style=+%T /var/tmp/a114r6/shots/ | grep "$1" | awk '{print $6, $7}'
