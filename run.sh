#!/bin/bash

set -e

debug_runtime=false
debug_user=false

while [ "$1" != "" ]; do
    case "$1" in
        --debug-runtime)
            debug_runtime=true
            shift
            ;;
        --debug-user)
            debug_user=true
            shift
            ;;
        *)
            break
            ;;
    esac
done

if [ "$debug_runtime" = "false" ]; then
    cargo build --release >/dev/null 2>&1
else
    cargo build >/dev/null 2>&1
fi

if [ "$debug_user" = "true" ]; then
    export ASAN_OPTIONS=detect_leaks=0
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
if [ "$debug_runtime" = "true" ]; then
    cgen_args="$cgen_args --debug-runtime"
fi
if [ "$debug_user" = "true" ]; then
    cgen_args="$cgen_args --debug-user"
fi
cd cgen
go run main.go $cgen_args
cd ..

cd $build_directory
ln -s ../cgen/predefined.h predefined.h
make -j
cd ..

$bin_file_name
