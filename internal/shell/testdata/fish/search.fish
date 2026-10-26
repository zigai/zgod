functions -e __zgod_preexec __zgod_postexec
set -g __test_line original
function commandline
    switch "$argv[1]"
        case -r
            set -g __test_line "$argv[2]"
        case -f
            if test "$argv[2]" = execute
                eval "$__test_line"
            end
        case '*'
            printf '%s' "$__test_line"
    end
end
__zgod_search
command "$__zgod_bin" observed "$__test_line"
