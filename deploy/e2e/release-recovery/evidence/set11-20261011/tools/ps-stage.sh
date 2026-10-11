#!/bin/bash
pgrep -af "stage-common|git -c|cp /mnt" | cut -c1-200
