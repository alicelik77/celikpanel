# copy the built escape kit (+ prt.sh) to a lab guest at /root/kit. usage: kitpush.sh CELL. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/env.sh
cp $A/prt.sh $B/kit/deploy/e2e/sandbox-escape/prt.sh; chmod 755 $B/kit/deploy/e2e/sandbox-escape/*.sh
tar -C $B/kit -cf - deploy/e2e/sandbox-escape | lab ssh $1 main -- "sudo -n sh -c 'rm -rf /root/kit; mkdir -p /root/kit; tar -xf - -C /root/kit; ls /root/kit/deploy/e2e/sandbox-escape /root/kit/deploy/e2e/sandbox-escape/bin'"
