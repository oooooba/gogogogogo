#!/bin/bash

set -ex

cargo fmt --all -- --check
cargo clippy --all-targets -- -D warnings

files=$(gofmt -l cgen xtests)
if [ -n "$files" ]; then
	echo need gofmt to "$files"
    exit 1
fi

clang-format --dry-run --Werror -i cgen/predefined.h
