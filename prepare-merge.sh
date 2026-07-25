#!/bin/bash

set -ex

cargo clippy --all-targets -- -D warnings
cargo fmt --all
gofmt -l cgen/*.go xtests/*.go
clang-format -i cgen/predefined.h

cargo test
cargo +nightly miri test
bash run_xtests.sh

go_root=$(go env GOROOT)
if [ ! -d "$go_root/test" ]; then
    exit 0
fi

set +ex

targets=(
    "array"
    "complit"
    "convert"
    "cplx0"
    "cplx1"
    "cplx2"
    "cplx5"
    "divmod"
    "embed"
    "for"
    "interbasic"
    "interfun"
    "intervar"
    "label"
    "litfun"
    "mfunc"
    "ptrfun"
    "ptrvar"
    "range"
    "rob1"
    "robfor"
    "robfunc"
    "shift"
    "simparray"
    "simpbool"
    "simpconv"
    "simpfun"
    "simpswitch"
    "simpvar"
    "slicearray"
    "sliceslice"
    "string"
    "strvar"
)

exit_status=0
for target in ${targets[@]}; do
    path=$go_root/test/ken/$target.go
    echo -n "[$path] "

    base=`basename $path`

    expect_result=/tmp/raw_expect_$base.txt
    actual_result=/tmp/raw_actual_$base.txt
    compare_result=/tmp/compare_$base.txt

    if ! go run $path >$expect_result 2>&1; then
        echo "FAIL (go run crashed)"
        exit_status=1
        continue
    fi
    if ! bash ./run.sh $path >$actual_result 2>&1; then
        echo "FAIL (run.sh crashed)"
        exit_status=1
        continue
    fi

    if diff -y $expect_result $actual_result >$compare_result; then
        echo PASS
    else
        echo FAIL
        cat $compare_result
        exit_status=1
    fi
done

exit $exit_status
