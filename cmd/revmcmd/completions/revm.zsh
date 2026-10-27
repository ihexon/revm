#compdef revm
_revm() {
    local -a args opts
    args=("${words[@]:1:$((CURRENT-2))}")
    [[ "${words[CURRENT]}" == -* ]] && args+=("${words[CURRENT]}")
    opts=("${(@f)$("${words[1]}" _complete "${args[@]}" 2>/dev/null)}")
    if [[ -n "${opts[1]}" ]]; then
        compadd -- "${opts[@]}"
    else
        _files
    fi
}
compdef _revm revm
