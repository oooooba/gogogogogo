#!/bin/bash

panic=false
build_args=()
path=""

for arg in "$@"; do
    case "$arg" in
        --panic)
            panic=true
            ;;
        --debug-user|--debug-runtime)
            build_args+=("$arg")
            ;;
        *)
            if [ -z "$path" ]; then
                path="$arg"
            else
                echo "unexpected argument: $arg" >&2
                exit 1
            fi
            ;;
    esac
done

if [ -z "$path" ]; then
    echo "usage: check_equivalence.sh [--panic] [--debug-user] [--debug-runtime] <path.go>" >&2
    exit 1
fi

tmp_dir="tmp/$path.$(IFS=; echo "${build_args[*]}").d"
mkdir -p "$tmp_dir"

expect_result="$tmp_dir/raw_expect.txt"
actual_result="$tmp_dir/raw_actual.txt"
compare_result="$tmp_dir/compare.txt"

build_args+=("-b")
build_args+=("$tmp_dir/build")

exit_status=0

if [ "$panic" = "true" ]; then
    if ! go run "$path" >$expect_result 2>&1; then
        if ! bash ./run.sh "${build_args[@]}" "$path" >$actual_result 2>&1; then
            if head -n 1 $expect_result | diff -y - $actual_result >$compare_result; then
                echo PASS
            else
                echo FAIL
                cat $compare_result
                exit_status=1
            fi
        else
            echo "FAIL (exit normally)"
            exit_status=1
        fi
    else
        echo "FAIL (go run exit normally)"
        exit_status=1
    fi
else
    if ! go run "$path" >$expect_result 2>&1; then
        echo "FAIL (go run crashed)"
        exit_status=1
    else
        bash ./run.sh "${build_args[@]}" "$path" >$actual_result 2>&1 || true
        if diff -y $expect_result $actual_result >$compare_result; then
            echo PASS
        else
            echo FAIL
            cat $compare_result
            exit_status=1
        fi
    fi
fi

exit $exit_status
