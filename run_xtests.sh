#!/bin/bash

set -e

run_args=("$@")

for path in $(find xtests -name '*.go' -type f | sort); do
    base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        continue
    fi

    if [[ $base == "bytes.go" || $base == "unicode.go" ]]; then
        skip=false
        for run_arg in "${run_args[@]}"; do
            if [ "$run_arg" == "--debug-user" ]; then
                skip=true
            fi
        done
        if [ "$skip" = "true" ]; then
            echo "[$path] SKIP ($base is too slow with --debug-user)"
            continue
        fi
    fi

    (
        check_args=()
        case $base in
            panic_*)
                check_args+=(--panic)
                ;;
        esac

        if ! bash ./check_equivalence.sh "${check_args[@]}" "${run_args[@]}" "$path"; then
            echo "[$path] FAIL"
            exit 1
        fi
    ) &
done

fail_count=0
for job in $(jobs -p); do
    if ! wait $job; then
        fail_count=$((fail_count+1))
    fi
done

if [ $fail_count -ne 0 ]; then
    exit 1
fi
