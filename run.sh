#!/bin/bash

set -e

mode=release
if [ "$1" = "--debug-runtime" ]; then
    mode=debug
    shift
fi

if [ "$mode" = "release" ]; then
    cargo build --release >/dev/null 2>&1
else
    cargo build >/dev/null 2>&1
fi

build_directory=build

if [ -d $build_directory ]; then
    rm -rf $build_directory
fi
mkdir $build_directory

bin_file_name=$build_directory/bin.exe

dir_name=$(cd `dirname $1` && pwd)
base_name=`basename $1`
src=$dir_name/$base_name
cgen_args="-b ../$build_directory -i $src"
if [ "$mode" = "debug" ]; then
    cgen_args="$cgen_args --debug-runtime"
fi
cd cgen
go run main.go $cgen_args
cd ..

cd $build_directory
ln -s ../cgen/predefined.h predefined.h
make -j
cd ..

$bin_file_name
