for s in d12 u22 ar; do echo "## $s"; grep -E "^==|install exit|terminal rc|^-- " /var/tmp/a129o/logs/setup127-$s.log | tail -5 | cut -c1-250; done
