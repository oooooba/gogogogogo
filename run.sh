#!/bin/bash

set -e

build_directory=build

if [ -d $build_directory ]; then
    rm -rf $build_directory
fi
mkdir $build_directory

bin_file_name=$build_directory/bin.exe

dir_name=$(cd `dirname $1` && pwd)
base_name=`basename $1`
src=$dir_name/$base_name
cd cgen
go run main.go -b ../$build_directory -i $src
cd ..

cd $build_directory
make -j
cd ..

$bin_file_name
