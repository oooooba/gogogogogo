#!/bin/bash

set -e

run_args=("$@")

function check_one() {
    local path="$1"
    shift
    local run_args=("$@")
    local base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        return 0
    fi

    if [[ $base == "bytes.go" || $base == "strings.go" || $base == "unicode.go" ]]; then
        local skip=false
        for run_arg in "${run_args[@]}"; do
            if [ "$run_arg" == "--debug-user" ]; then
                skip=true
            fi
        done
        if [ "$skip" = "true" ]; then
            echo "[$path] SKIP ($base is too slow with --debug-user)"
            return 0
        fi
    fi

    local check_args=()
    case $base in
        panic_*)
            check_args+=(--panic)
            ;;
    esac

    if ! bash ./check_equivalence.sh "${check_args[@]}" "${run_args[@]}" "$path"; then
        echo "[$path] FAIL"
        return 1
    fi

    return 0
}

export -f check_one

find xtests -name '*.go' -type f | sort | xargs -P 3 -I {} bash -c 'check_one "$@"' _ {} "${run_args[@]}"
