#!/bin/bash

set -ex

cargo fmt --all
cargo clippy --all-targets -- -D warnings
gofmt -l cgen xtests
clang-format -i cgen/predefined.h
