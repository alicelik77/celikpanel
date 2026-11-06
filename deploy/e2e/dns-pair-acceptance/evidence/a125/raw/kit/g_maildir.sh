# guest (root, read-only): subjects of the messages in a mailbox. $1 address
U=${1%@*}; D=${1#*@}
postqueue -f 2>/dev/null; sleep 2
for f in $(find /var/mail /var/vmail /home/vmail -xdev -type f -path "*$D*$U*" 2>/dev/null | grep -vE 'dovecot|maildirsize|subscriptions'); do grep -m1 '^Subject:' "$f" | sed 's/^/  /'; done | sort | uniq -c
echo "  queue: $(postqueue -j | wc -l)"
