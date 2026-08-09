#!/bin/bash

set -e

mkdir -p tmp

run_args=("$@")

exit_status=0
for path in $(find xtests -name '*.go' -type f | sort); do
    base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        continue
    fi

    if [ "$base" == "unicode.go" ]; then
        skip=false
        for run_arg in "${run_args[@]}"; do
            if [ "$run_arg" == "--debug-user" ]; then
                skip=true
            fi
        done
        if [ "$skip" = "true" ]; then
            echo "[$path] SKIP (unicode.go is too slow with --debug-user)"
            continue
        fi
    fi

    echo -n "[$path] "

    check_args=()
    case $base in
        panic_*)
            check_args+=(--panic)
            ;;
    esac

    if ! bash ./check_equivalence.sh "${check_args[@]}" "${run_args[@]}" "$path"; then
        exit_status=1
    fi
done

exit $exit_status
