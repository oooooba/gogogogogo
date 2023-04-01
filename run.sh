#!/bin/bash

set -e

c_file_name=/tmp/out.c
bin_file_name=/tmp/bin.exe

dir_name=$(cd `dirname $1` && pwd)
base_name=`basename $1`
src=$dir_name/$base_name
cd cgen
go run main.go -i $src >$c_file_name
cd ..

gcc -Wall -Wextra -Werror -std=c11 -g \
    -fstrict-aliasing -Wstrict-aliasing \
    -fsanitize=undefined -fno-sanitize-recover=all \
    -o $bin_file_name $c_file_name target/debug/libgogogogogo.a \
    -lpthread -ldl -lm \

$bin_file_name
