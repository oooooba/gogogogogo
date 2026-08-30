#!/bin/bash

go_root=$(go env GOROOT)
if [ ! -d "$go_root/test" ]; then
    exit 0
fi

set +ex

targets=(
    "array"
    "complit"
    "convert"
    "cplx0"
    "cplx1"
    "cplx2"
    "cplx5"
    "divconst"
    "divmod"
    "embed"
    "for"
    "interbasic"
    "interfun"
    "intervar"
    "label"
    "litfun"
    "mfunc"
    "modconst"
    "ptrfun"
    "ptrvar"
    "range"
    "rob1"
    "robfor"
    "robfunc"
    "shift"
    "simparray"
    "simpbool"
    "simpconv"
    "simpfun"
    "simpswitch"
    "simpvar"
    "slicearray"
    "sliceslice"
    "string"
    "strvar"
)

function check_ken() {
    local target="$1"
    local path=$go_root/test/ken/$target.go

    if ! bash ./check_equivalence.sh "$path"; then
        echo "[$path] FAIL"
        return 1
    fi

    return 0
}

export -f check_ken
export go_root

fail_count=0
if ! printf '%s\n' "${targets[@]}" | xargs -P 3 -I {} bash -c 'check_ken "$@"' _ {}; then
    fail_count=1
fi

if [ $fail_count -eq 0 ]; then
    echo PASS
else
    echo FAIL
fi

exit $fail_count
