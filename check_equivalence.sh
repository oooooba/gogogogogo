#!/bin/bash

panic=false
build_args=()
build_dir=""
path=""

while [ $# -gt 0 ]; do
    case "$1" in
        --panic)
            panic=true
            shift
            ;;
        --debug-user|--debug-runtime)
            build_args+=("$1")
            shift
            ;;
        -b)
            if [ -z "$2" ]; then
                echo "option -b requires an argument" >&2
                exit 1
            fi
            build_dir="$2"
            shift 2
            ;;
        *)
            if [ -z "$path" ]; then
                path="$1"
            else
                echo "unexpected argument: $1" >&2
                exit 1
            fi
            shift
            ;;
    esac
done

if [ -z "$path" ]; then
    echo "usage: check_equivalence.sh [--panic] [--debug-user] [--debug-runtime] [-b <build_dir>] <path.go>" >&2
    exit 1
fi

tmp_dir="tmp/$path.$(IFS=; echo "${build_args[*]}").d"
mkdir -p "$tmp_dir"

expect_result="$tmp_dir/raw_expect.txt"
actual_result="$tmp_dir/raw_actual.txt"
compare_result="$tmp_dir/compare.txt"

if [ -n "$build_dir" ]; then
    build_args+=("-b")
    build_args+=("$build_dir")
fi

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
