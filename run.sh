#!/bin/bash

set -e

debug_user=false

build_args=()
for arg in "$@"; do
    case "$arg" in
        --debug-user)
            debug_user=true
            build_args+=("$arg")
            ;;
        --debug-runtime)
            build_args+=("$arg")
            ;;
        *)
            build_args+=("$arg")
            ;;
    esac
done

bin_file=$(bash ./build.sh "${build_args[@]}")

if [ "$debug_user" = "true" ]; then
    export ASAN_OPTIONS=detect_leaks=0
fi

$bin_file
