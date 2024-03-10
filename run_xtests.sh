#!/bin/bash

set -e

cargo build

exit_status=0
for path in xtests/*; do
    echo -n "[$path] "

    base=`basename $path`

    if [ $base == "reflect.go" ]; then
        continue
    fi

    expect_result=/tmp/raw_expect_$base.txt
    actual_result=/tmp/raw_actual_$base.txt

    case $base in
        panic_*)
            go run $path 2>&1 | head -n 1 >$expect_result || true
            if ! bash ./run.sh $path >$actual_result 2>&1; then
                if diff -y $expect_result $actual_result >$compare_result; then
                    echo PASS
                else
                    echo FAIL
                    cat $compare_result
                    exit_status=1
                fi
            else
                echo "FAIL (exit normaly)"
                exit_status=1
            fi
            ;;
        *)
            go run $path >$expect_result 2>&1
            bash ./run.sh $path >$actual_result 2>&1
            compare_result=/tmp/compare_$base.txt
            if diff -y $expect_result $actual_result >$compare_result; then
                echo PASS
            else
                echo FAIL
                cat $compare_result
                exit_status=1
            fi
            ;;
        esac
done

exit $exit_status
