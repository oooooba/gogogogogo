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

build_directory=build

if [ -d $build_directory ]; then
    rm -rf $build_directory
fi
mkdir $build_directory

dir_name=$(cd `dirname $1` && pwd)
base_name=`basename $1`
src=$dir_name/$base_name
cgen_args="-b ../$build_directory -i $src"
cd cgen
go run main.go $cgen_args
cd ..

cd $build_directory
if [ "$debug_user" = "true" ] && [ "$debug_runtime" = "true" ]; then
    make -j bin-debug-user-debug-runtime.exe
    bin_file_name=bin-debug-user-debug-runtime.exe
elif [ "$debug_user" = "true" ]; then
    make -j bin-debug-user.exe
    bin_file_name=bin-debug-user.exe
elif [ "$debug_runtime" = "true" ]; then
    make -j bin-debug-runtime.exe
    bin_file_name=bin-debug-runtime.exe
else
    make -j bin.exe
    bin_file_name=bin.exe
fi
cd ..

echo $build_directory/$bin_file_name
