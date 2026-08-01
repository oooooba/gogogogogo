#!/bin/bash

set -e

for path in $(find xtests -name '*.go' -type f | sort); do
    base=`basename $path`

    if [ "$base" == "reflect.go" ]; then
        continue
    fi

    echo -n "[$path] "

    bin1=$(bash ./build.sh $path)
    hash1=$(sha256sum $bin1 | awk '{print $1}')

    bin2=$(bash ./build.sh $path)
    hash2=$(sha256sum $bin2 | awk '{print $1}')

    if [ "$hash1" == "$hash2" ]; then
        echo "PASS"
    else
        echo "FAIL"
        echo "  first:  $hash1"
        echo "  second: $hash2"
        exit 1
    fi
done
