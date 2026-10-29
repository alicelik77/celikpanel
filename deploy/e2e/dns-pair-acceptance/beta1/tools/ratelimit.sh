# on labnet: slow the main's package downloads (lab network condition) so the backup really waits for the main's DNS
# usage: ratelimit.sh on|off IP
nft delete table ip beta1slow 2>/dev/null
if [ "$1" = on ]; then
cat > /root/beta1/slow.nft <<NFT
table ip beta1slow {
    chain fwd {
        type filter hook forward priority -5; policy accept;
        iifname "up0" ip daddr $2 limit rate over 12 kbytes/second counter drop
    }
}
NFT
nft -f /root/beta1/slow.nft
fi
nft list table ip beta1slow 2>/dev/null || echo "no rate limit"
