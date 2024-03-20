use std::mem;

use crate::object::slice::SliceObject;
use crate::object::string::StringObject;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameStringNewFromByteSlice<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    byte_slice: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_string_new_from_byte_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringNewFromByteSlice>();
    let len = frame.byte_slice.size();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    let src_bytes = frame.byte_slice.as_bytes(mem::size_of::<u8>());
    builder.append_bytes(src_bytes);

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromByteSlice>();
    *frame.result_ptr = builder.build();

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringNewFromRune<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    rune: usize,
}

#[no_mangle]
pub extern "C" fn gox5_string_new_from_rune(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringNewFromRune>();
    let rune = frame.rune;

    assert!(rune <= std::u32::MAX as usize);
    let ch = char::from_u32(rune as u32).unwrap();
    let len = ch.len_utf8();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_char(ch);

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRune>();
    *frame.result_ptr = builder.build();

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringNewFromRuneSlice<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    rune_slice: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_string_new_from_rune_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let stack_frame = ctx.stack_frame::<StackFrameStringNewFromRuneSlice>();
    let rune_slice = &stack_frame.rune_slice;

    let len = {
        let elem_size = mem::size_of::<u32>();
        let src_bytes = rune_slice.as_bytes(elem_size);
        src_bytes[..rune_slice.size() * elem_size]
            .chunks(elem_size)
            .fold(0, |acc, bytes| {
                let rune = u32::from_le_bytes(bytes.try_into().unwrap());
                let ch = char::from_u32(rune).unwrap();
                acc + ch.len_utf8()
            })
    };

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    let elem_size = mem::size_of::<u32>();
    let src_bytes = rune_slice.as_bytes(elem_size);
    src_bytes[..rune_slice.size() * elem_size]
        .chunks(elem_size)
        .for_each(|bytes| {
            let rune = u32::from_le_bytes(bytes.try_into().unwrap());
            let ch = char::from_u32(rune).unwrap();
            builder.append_char(ch);
        });

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRuneSlice>();
    *frame.result_ptr = builder.build();

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringAppend<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    lhs: StringObject,
    rhs: StringObject,
}

#[no_mangle]
pub extern "C" fn gox5_string_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringAppend>();
    let len = frame.lhs.len_in_bytes() + frame.rhs.len_in_bytes();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_bytes(frame.lhs.as_bytes());
    builder.append_bytes(frame.rhs.as_bytes());

    let frame = ctx.stack_frame_mut::<StackFrameStringAppend>();
    *frame.result_ptr = builder.build();

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringLength<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    string: StringObject,
}

#[no_mangle]
pub extern "C" fn gox5_string_length(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringLength>();
    let result = isize::try_from(frame.string.len_in_bytes()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameStringLength>();
    *frame.result_ptr = result;

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringNext<'a> {
    common: StackFrameCommon,
    string: StringObject,
    index: Option<&'a mut isize>,
    rune: Option<&'a mut i32>,
    found: &'a mut bool,
    count: &'a mut usize,
}

#[no_mangle]
pub extern "C" fn gox5_string_next(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameStringNext>();

    let s = frame.string.to_str().unwrap();
    let index = *frame.count;
    if let Some(c) = s.chars().nth(index) {
        if let Some(p) = frame.index.as_mut() {
            **p = index as isize;
        }
        if let Some(p) = frame.rune.as_mut() {
            **p = c as i32;
        }
        *frame.found = true;
        *frame.count = index + 1;
    } else {
        *frame.found = false;
    }

    ctx.leave()
}

#[repr(C)]
struct StackFrameStringSubstr<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    base: StringObject,
    low: isize,
    high: isize,
}

#[no_mangle]
pub extern "C" fn gox5_string_substr(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringSubstr>();

    let low = {
        let low = frame.low;
        if low < 0 {
            assert_eq!(low, -1);
            0
        } else {
            low as usize
        }
    };

    let high = {
        let high = frame.high;
        if high < 0 {
            assert_eq!(high, -1);
            frame.base.len_in_bytes()
        } else {
            high as usize
        }
    };

    assert!(low <= high);
    let len = high - low;

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_bytes(&frame.base.as_bytes()[low..high]);

    let frame = ctx.stack_frame_mut::<StackFrameStringSubstr>();
    *frame.result_ptr = builder.build();

    ctx.leave()
}
