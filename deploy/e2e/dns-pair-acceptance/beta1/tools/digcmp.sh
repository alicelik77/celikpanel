# usage on labnet: digcmp.sh ZONE IP1 IP2 NAMES...   compares authoritative answers of both servers (UDP and TCP)
zone=$1; a=$2; b=$3; shift 3
for n in "$@"; do for t in SOA NS A AAAA CNAME MX TXT; do
  for proto in +notcp +tcp; do
    x=$(dig +norec $proto +time=2 +tries=1 @$a $n $t | awk '/^;; ->>HEADER/{print $6} /flags:/{for(i=1;i<=NF;i++) if($i=="aa") aa="aa"} END{}' | tr -d ',' | head -1)
    ra=$(dig +norec $proto +short +time=2 +tries=1 @$a $n $t | sort | tr '\n' '|'); rb=$(dig +norec $proto +short +time=2 +tries=1 @$b $n $t | sort | tr '\n' '|')
    fa=$(dig +norec $proto +time=2 +tries=1 @$a $n $t | grep -o 'flags: [a-z ]*' | head -1); fb=$(dig +norec $proto +time=2 +tries=1 @$b $n $t | grep -o 'flags: [a-z ]*' | head -1)
    [ -z "$ra$rb" ] && continue
    same=DIFF; [ "$ra" = "$rb" ] && same=same
    echo "$n $t $proto main[$fa]=$ra backup[$fb]=$rb $same"
  done; done; done
