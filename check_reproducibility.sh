#!/bin/bash

set -e

tmp_dir=tmp/reproducibility
mkdir -p $tmp_dir

for path in $(find xtests -name '*.go' -type f | sort); do
    base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        continue
    fi

    (
        build_dir="$tmp_dir/${path#xtests/}.d"

        if ! bin1=$(bash ./build.sh -b $build_dir $path); then
            echo "[$path] FAIL (build failed)"
            exit 1
        fi
        hash1=$(sha256sum $bin1 | awk '{print $1}')

        if ! bin2=$(bash ./build.sh -b $build_dir $path); then
            echo "[$path] FAIL (build failed)"
            exit 1
        fi
        hash2=$(sha256sum $bin2 | awk '{print $1}')

        if [ "$hash1" == "$hash2" ]; then
            echo "[$path] PASS"
        else
            echo "[$path] FAIL"
            echo "  first:  $hash1"
            echo "  second: $hash2"
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
