#!/bin/bash

set -e

cargo build

exit_status=0
for path in xtests/*; do
    echo -n "[$path] "

    base=`basename $path`

    expect_result=/tmp/raw_expect_$base.txt
    go run $path >$expect_result
    actual_result=/tmp/raw_actual_$base.txt
    bash ./run.sh $path >$actual_result

    compare_result=/tmp/compare_$base.txt
    if diff -y $expect_result $actual_result >$compare_result; then
        echo PASS
    else
        echo FAIL
        cat $compare_result
        exit_status=1
    fi
done

exit $exit_status
