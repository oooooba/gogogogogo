#!/bin/bash

set -e

tmp_dir=tmp/reproducibility
rm -rf $tmp_dir
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

verify_package_c() {
    local group_dir=$(mktemp -d)
    local fail_count=0
    local package_count=0
    local file_count=0

    while IFS= read -r cfile; do
        local base=$(basename "$cfile")
        case "$base" in
            package_*.c) ;;
            *) continue ;;
        esac

        if [ "$base" == "package_command_2D_line_2D_arguments.c" ]; then
            continue
        fi

        local pkg_name="${base#package_}"
        pkg_name="${pkg_name%.c}"
        local file_hash=$(sha256sum "$cfile" | awk '{print $1}')
        file_count=$((file_count+1))

        local group_file="$group_dir/$pkg_name"
        if [ ! -f "$group_file" ]; then
            echo "$file_hash  $cfile" > "$group_file"
            package_count=$((package_count+1))
        else
            local first_hash=$(awk '{print $1}' "$group_file")
            if [ "$first_hash" != "$file_hash" ]; then
                echo "[package_c] MISMATCH: $pkg_name"
                echo "  first:  $(cat "$group_file")"
                echo "  second: $file_hash  $cfile"
                fail_count=$((fail_count+1))
            fi
        fi
    done < <(find -L "$tmp_dir" -name 'package_*.c' -type f | sort)

    rm -rf "$group_dir"

    if [ $fail_count -ne 0 ]; then
        echo "[package_c] $fail_count group(s) differ ($package_count packages, $file_count files)"
        return 1
    fi

    echo "[package_c] VERIFIED ($package_count packages, $file_count files)"
    return 0
}

if ! verify_package_c; then
    exit 1
fi
