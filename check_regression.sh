#!/bin/bash

set -ex

rm -rf cgen/cache
mkdir -p cgen/cache
(cd cgen && go run main.go -b ../cgen/cache -i supported_packages.go)
rm -f cgen/cache/shared_definition.c cgen/cache/predefined.h cgen/cache/Makefile
rm -f cgen/cache/package_command_2D_line_2D_arguments.c

mkdir -p tmp

cargo test
cargo +nightly miri test
bash run_xtests.sh
