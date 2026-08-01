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
