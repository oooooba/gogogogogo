#!/bin/bash

set -e

c_file_name=/tmp/out.c
bin_file_name=/tmp/bin.exe

cd cgen
go run main.go -i ../xtests/`basename $1` >$c_file_name
cd ..

gcc -Wall -Wextra -Werror -std=c11 \
    -o $bin_file_name $c_file_name target/debug/libgogogogogo.a \
    -lpthread -ldl -lm \

$bin_file_name
