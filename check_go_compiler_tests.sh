#!/bin/bash

go_root=$(go env GOROOT)
if [ ! -d "$go_root/test" ]; then
    exit 0
fi

set +ex

targets=( 
    "235.go"
    "alg.go"
    "alias1.go"
    "align.go"
    "armimm.go"
    "atomicload.go"
    "bom.go"
    "chan/fifo.go"
    "chan/goroutines.go"
    "chan/powser1.go"
    "chan/powser2.go"
    "chan/select.go"
    "chan/select4.go"
    "chan/select6.go"
    "chan/select7.go"
    "chan/select8.go"
    "chan/sendstmt.go"
    "chan/sieve1.go"
    "chan/sieve2.go"
    "chan/zerosize.go"
    "char_lit.go"
    "checkbce.go"
    "clear.go"
    "closure1.go"
    "closure2.go"
    "closure7.go"
    "complit.go"
    "const3.go"
    "const4.go"
    "const8.go"
    "convert.go"
    "copy.go"
    "crlf.go"
    "decl.go"
    "defer.go"
    "devirt.go"
    "escape.go"
    "escape3.go"
    "float_lit.go"
    "float_lit2.go"
    "floatcmp.go"
    "for.go"
    "func.go"
    "func5.go"
    "func6.go"
    "func7.go"
    "func8.go"
    "fuse.go"
    "gc1.go"
    "helloworld.go"
    "if.go"
    "indirect.go"
    "initcomma.go"
    "int_lit.go"
    "intcvt.go"
    "interface/bigdata.go"
    "interface/convert.go"
    "interface/convert1.go"
    "interface/convert2.go"
    "interface/embed.go"
    "interface/receiver.go"
    "interface/struct.go"
    "iota.go"
    "ken"
    "linkx.go"
    "literal.go"
    "literal2.go"
    "loopbce.go"
    "map.go"
    "mapclear.go"
    "mergemul.go"
    "method3.go"
    "named.go"
    "newexpr.go"
    "nilptr4.go"
    "noinit.go"
    "nul1.go"
    "phiopt.go"
    "printbig.go"
    "prove_popcount.go"
    "range3.go"
    "reorder2.go"
    "simassign.go"
    "sizeof.go"
    "solitaire.go"
    "strcopy.go"
    "strength.go"
    "stringrange.go"
    "turing.go"
    "typeswitch.go"
    "typeswitch1.go"
    "unsafe_slice_data.go"
    "unsafe_string.go"
    "unsafe_string_data.go"
    "utf.go"
    "varinit.go"
)

function run_file() {
    local path="$1"
    if ! bash ./check_equivalence.sh "$path"; then
        echo "[$path] FAIL"
        return 1
    fi
    return 0
}

function check_target() {
    local target="$1"
    local root="$go_root/test"

    # A bare "Directory" (no ".go") runs every test file under it one by one.
    if [[ "$target" != *.go ]] && [ -d "$root/$target" ]; then
        local rc=0
        for f in "$root/$target"/*.go; do
            if ! run_file "$f"; then
                rc=1
            fi
        done
        return $rc
    fi

    # Anything else names a single test file, relative to $go_root/test.
    run_file "$root/$target"
    return $?
}

export -f check_target
export -f run_file
export go_root

fail_count=0
if ! printf '%s\n' "${targets[@]}" | xargs -P 3 -I {} bash -c 'check_target "$@"' _ {}; then
    fail_count=1
fi

if [ $fail_count -eq 0 ]; then
    echo PASS
else
    echo FAIL
fi

exit $fail_count
