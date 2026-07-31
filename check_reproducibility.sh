#!/bin/bash

set -e

combinations=(
    ""
    "--debug-runtime"
    "--debug-user"
    "--debug-user --debug-runtime"
)

for combo in "${combinations[@]}"; do
    bin1=$(bash ./build.sh $combo xtests/generics.go)
    hash1=$(sha256sum $bin1 | awk '{print $1}')

    bin2=$(bash ./build.sh $combo xtests/generics.go)
    hash2=$(sha256sum $bin2 | awk '{print $1}')

    if [ "$hash1" == "$hash2" ]; then
        echo "[$combo] PASS: $hash1"
    else
        echo "[$combo] FAIL"
        echo "  first:  $hash1"
        echo "  second: $hash2"
        exit 1
    fi
done
