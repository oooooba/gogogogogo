#!/bin/bash

set -e

tmp_dir=tmp/reproducibility
mkdir -p $tmp_dir

function check_one() {
    local path="$1"
    local base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        return 0
    fi

    build_dir="$tmp_dir/${path#xtests/}.d"

    if ! bin1=$(bash ./build.sh -b $build_dir $path); then
        echo "[$path] FAIL (build failed)"
        return 1
    fi
    hash1=$(sha256sum $bin1 | awk '{print $1}')

    if ! bin2=$(bash ./build.sh -b $build_dir $path); then
        echo "[$path] FAIL (build failed)"
        return 1
    fi
    hash2=$(sha256sum $bin2 | awk '{print $1}')

    if [ "$hash1" == "$hash2" ]; then
        echo "[$path] PASS"
        return 0
    else
        echo "[$path] FAIL"
        echo "  first:  $hash1"
        echo "  second: $hash2"
        return 1
    fi
}

export -f check_one
export tmp_dir

find xtests -name '*.go' -type f | sort | xargs -P 3 -I {} bash -c 'check_one "$@"' _ {}
