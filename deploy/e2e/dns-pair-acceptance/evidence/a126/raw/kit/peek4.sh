source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/env.sh
infra delegate p6.test ns1.p6.test 192.0.3.190 ns2.p6.test 192.0.3.191
lab ssh beta1-lab labnet -- "sudo -n sh -c 'dig +norec @192.0.2.53 p6.test NS | grep status; dig @192.0.2.53 frankfurt.p6.test A +short; dig @192.0.2.53 boston.p6.test A +short'"
