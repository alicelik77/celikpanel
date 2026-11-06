source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/env.sh
source $H/common.sh
lab ssh beta1-lab labnet -- "dig +short @192.0.3.190 frankfurt.p6.test A; echo ---; dig +short @192.0.3.191 boston.p6.test A; echo ---; dig @127.0.0.1 frankfurt.p6.test A | grep -E 'status|^frank'; dig @127.0.0.1 p6.test NS | grep -E 'status|NS'; dig +norec @127.0.0.1 p6.test NS | grep -E 'status|NS|AUTH' | head; cat /etc/resolv.conf | head -3"
b1 sh p6 main "resolvectl status 2>/dev/null | grep -i 'dns server' | head -3; cat /etc/resolv.conf | grep -v '^#'; getent hosts frankfurt.p6.test; dig +short frankfurt.p6.test 2>&1 | head -2" --label peek | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
