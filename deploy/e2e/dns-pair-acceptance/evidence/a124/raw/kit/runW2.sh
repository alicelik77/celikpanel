# alpha.124 wave 2: fresh Debian 12, Ubuntu 22.04 (BIND; PowerDNS refused there) and Ubuntu 24.04 (BIND). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit
FIRST=${FIRST:-d12,u22}
case ",$FIRST," in *,d12,*) ( CELL=d12 IP=192.0.3.193 Z=d12.test OS=debian12 MEM=3072 SHORT=frankfurt DNSENGINE=bind bash $A/single124.sh > $A/singleD12.out 2>&1; bash $A/R2d12.sh > $A/R2d12.out 2>&1 ) & sleep 30;; esac
case ",$FIRST," in *,u22,*) ( CELL=u22 IP=192.0.3.194 Z=u22.test OS=ubuntu2204 MEM=3072 SHORT=boston DNSENGINE=bind bash $A/single124.sh > $A/singleU22.out 2>&1 ) & sleep 30;; esac
case ",$FIRST," in *,u24,*) ( CELL=u24 IP=192.0.3.195 Z=u24.test OS=ubuntu2404 MEM=3072 SHORT=boston DNSENGINE=bind bash $A/single124.sh > $A/singleU24.out 2>&1 ) &;; esac
wait
echo "wave2 $FIRST done $(date -u +%T)"
