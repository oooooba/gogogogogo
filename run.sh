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
rc=$?

# build.sh places the build directory in $TMPDIR (usually /tmp) via mktemp and
# never removes it. Those directories are large, so they eventually fill the
# (often small) tmpfs and later runs fail with "No rule to make target 'bin.exe'"
# because cgen cannot write the generated .c files. Remove the auto-generated
# build directory after the binary runs, but only when the caller did not pass
# an explicit -b build directory (which we must not delete).
if ! printf '%s\n' "${build_args[@]}" | grep -qx -- "-b"; then
    build_dir="$(dirname "$bin_file")"
    case "$build_dir" in
        /tmp/*) rm -rf "$build_dir" ;;
    esac
fi

exit $rc
