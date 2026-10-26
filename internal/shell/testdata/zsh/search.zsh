LBUFFER='original'
RBUFFER=''
zle() {
    if [[ "$1" == accept-line ]]; then
        eval "$LBUFFER"
    fi
}
__zgod_search_widget
"$__zgod_bin" observed "$LBUFFER$RBUFFER"
wait
