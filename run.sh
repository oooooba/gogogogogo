#!/bin/bash

set -e

debug_user=false

build_args=()
while [ $# -gt 0 ]; do
    case "$1" in
        --debug-user)
            debug_user=true
            build_args+=("$1")
            shift
            ;;
        --debug-runtime)
            build_args+=("$1")
            shift
            ;;
        -b)
            build_args+=("-b")
            build_args+=("$2")
            shift 2
            ;;
        *)
            build_args+=("$1")
            shift
            ;;
    esac
done

bin_file=$(bash ./build.sh "${build_args[@]}")

if [ "$debug_user" = "true" ]; then
    export ASAN_OPTIONS=detect_leaks=0
fi

$bin_file
