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

base=$(basename "$path")

mkdir -p tmp

expect_result=tmp/raw_expect_$base.txt
actual_result=tmp/raw_actual_$base.txt
compare_result=tmp/compare_$base.txt

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
