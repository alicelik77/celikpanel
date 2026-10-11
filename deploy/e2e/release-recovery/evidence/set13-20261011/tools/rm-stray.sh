#!/bin/bash
# set13: remove the empty file /.set13-restore that the mis-quoted inline command of 09:47:58Z created (only if empty).
[ -f /.set13-restore ] && [ ! -s /.set13-restore ] && rm -f /.set13-restore && echo "removed /.set13-restore (empty, created by set13 at 09:47:58Z)"
ls -la /.set13-restore 2>&1
