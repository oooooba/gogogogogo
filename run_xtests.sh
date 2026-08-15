#!/bin/bash

set -e

run_args=("$@")

function check_one() {
    local path="$1"
    shift
    local run_args=("$@")
    local base=`basename $path`

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
