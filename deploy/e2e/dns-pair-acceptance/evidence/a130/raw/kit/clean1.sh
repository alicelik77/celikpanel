# lab: drop builds no longer needed (2739ae3e9 label 130, fc79ec750 label 131); keep 129 and the image caches
B=/var/tmp/a130o
for c in 2739ae3e9e2755676b4508002d0ed641d04dbdc5 fc79ec7507f15dfbb86ea05a35311b464ba699a7; do rm -rf /var/tmp/cp-pair-accept/dist/$c-acceptance-license $B/tools-$c $B/npmci-$c.log $B/webbuild-$c.log $B/budget-$c.log; done
rm -rf $B/repo-v0.1.0-alpha.130 $B/repo-v0.1.0-alpha.131
du -sh /var/tmp/cp-pair-accept $B 2>/dev/null; ls /var/tmp/cp-pair-accept/dist
