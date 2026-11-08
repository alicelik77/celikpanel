# B lane item 6: Dovecot + Postfix on the joined server, then their removal takes CelikPanel's files and names an
# owner-edited one. usage: b_mail.sh PART... (install files ownedit impact remove). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B6leftovers-$CELL.log) 2>&1
for part in "$@"; do case $part in
install) echo "== install Dovecot $(ts)"; installc $N dovecot; grun $N g_mailfiles.sh ;;
ownedit) echo "== owner edits $(ts)"; grun $N g_ownmail.sh ;;
files) echo "== files $(ts)"; grun $N g_mailfiles.sh ;;
impact) for s in dovecot postfix; do echo "== removal impact $s $(ts)"; admn $N GET "/api/v1/service/removal-impact?service_id=$s" --label impact-$s | sbrief 1800; done ;;
remove) for s in ${SVCS:-dovecot postfix}; do echo "== remove $s $(ts)"; admn $N POST /api/v1/service/uninstall "{\"service_id\":\"$s\"}" --label rm-$s --timeout 900 | sbrief 1800; sleep 3; done; grun $N g_mailfiles.sh; audit "uninstall|leftover|removal" 20 ;;
esac; done
echo "== b_mail $* done $(ts)"
