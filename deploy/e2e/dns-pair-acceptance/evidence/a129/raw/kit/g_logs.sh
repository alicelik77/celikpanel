# guest (root): lab load for log retention. usage: g_logs.sh DOMAIN OTHERDOMAIN make|list. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
D=$1; O=$2; G=/var/log/nginx
case $3 in
make)
  ls -la $G | grep -E "$D|$O" | head
  for f in $D-access.log.1 $D-access.log.2.gz $D-error.log-20261001.gz $D-access.log.9.gz $O-access.log.2.gz $D-access.log.bak; do echo "lab old $f" > $G/$f; touch -d '20 days ago' $G/$f; done
  echo "lab young" > $G/$D-access.log.3.gz; touch -d '2 days ago' $G/$D-access.log.3.gz
  ln -sf /etc/hostname $G/$D-access.log.4.gz; touch -h -d '20 days ago' $G/$D-access.log.4.gz
  [ -f $G/$D-access.log ] && touch -d '20 days ago' $G/$D-access.log
  ;;
esac
echo "-- $G for $D and $O:"; ls -la --time-style=+%F $G | grep -E "$D|$O" | awk '{print $1, $5, $6, $7, $8, $9}'
