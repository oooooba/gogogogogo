#!/bin/bash

set -e

build_directory=build

if [ -d $build_directory ]; then
    rm -rf $build_directory
fi
mkdir $build_directory

c_file_name=$build_directory/out.c
bin_file_name=$build_directory/bin.exe

dir_name=$(cd `dirname $1` && pwd)
base_name=`basename $1`
src=$dir_name/$base_name
cd cgen
go run main.go -b ../$build_directory -i $src
cd ..

gcc -Wall -Wextra -Werror -std=c11 -g \
    -fstrict-aliasing -Wstrict-aliasing \
    -fsanitize=undefined -fno-sanitize-recover=all \
    -o $bin_file_name $c_file_name target/debug/libgogogogogo.a \
    -lpthread -ldl -lm \

$bin_file_name
