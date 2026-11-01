for f in "$@"; do echo "##### $f"; tail -n ${N:-12} /var/tmp/a114r3/$f | cut -c1-260; done
free -g | head -2
