# Requires bash-completion.
_revm() {
    local cur prev words cword opts
    _get_comp_words_by_ref -n =: cur prev words cword || return
    local -a args=("${words[@]:1:cword-1}")
    [[ "$cur" == -* ]] && args+=("$cur")
    opts=$("${words[0]}" _complete "${args[@]}" 2>/dev/null)
    COMPREPLY=($(compgen -W "$opts" -- "$cur"))
}
complete -o default -F _revm revm
