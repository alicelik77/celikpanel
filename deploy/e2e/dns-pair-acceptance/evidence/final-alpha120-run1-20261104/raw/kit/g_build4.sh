u=celikrtbuild-1.service
systemctl show $u -p ExecStart,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainCode,ExecMainStatus,User,WorkingDirectory,BindPaths,BindReadOnlyPaths,ReadWritePaths,TemporaryFileSystem,ProtectControlGroupsEx,ProtectControlGroups,RemainAfterExit,Type,StandardOutput,LogNamespace,ProtectSystem,ProtectHome,InaccessiblePaths,NoExecPaths,ExecPaths 2>&1 | cut -c1-400
journalctl --no-pager -o short-precise --since "02:56:44" --until "02:56:57" 2>/dev/null | grep -vE "sshd|pam_unix|sudo|logind|session" | head -20 | cut -c1-300
