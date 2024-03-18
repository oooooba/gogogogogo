use std::cmp;
use std::iter;
use std::mem;
use std::ptr;

use crate::object::slice::SliceObject;
use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectAllocator;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameSliceFromString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    type_id: TypeId,
    src: StringObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_from_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceFromString>();

    let elem_size = frame.type_id.size();
    assert!(elem_size == mem::size_of::<u8>() || elem_size == mem::size_of::<u32>());

    let len = frame.src.len_in_bytes();
    let buffer_size = len * elem_size;
    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(buffer_size, |_ptr| {}));

    let mut result = SliceObject::new(ptr, len, len);
    if frame.type_id.size() == mem::size_of::<u8>() {
        result
            .as_bytes_mut(elem_size)
            .clone_from_slice(frame.src.as_bytes());
    } else {
        let s = frame.src.to_str().unwrap();
        iter::zip(
            result.as_bytes_mut(elem_size)[..buffer_size].chunks_mut(elem_size),
            s.chars(),
        )
        .for_each(|(dst_bytes, ch)| {
            let src_bytes = (ch as u32).to_le_bytes();
            dst_bytes.clone_from_slice(&src_bytes);
        });
    }

    let frame = ctx.stack_frame_mut::<StackFrameSliceFromString>();
    *frame.result_ptr = result;

    ctx.leave()
}

fn reallocate_slice(
    base: &SliceObject,
    elem_size: usize,
    extend_bytes: &[u8],
    allocator: &mut dyn ObjectAllocator,
) -> SliceObject {
    assert!(elem_size > 0);
    assert!(extend_bytes.len() % elem_size == 0);

    let new_size = base.size() + extend_bytes.len() / elem_size;
    let mut result = if new_size > base.capacity() {
        let new_capacity = new_size * 2;
        let buffer_size = new_capacity * elem_size;
        let ptr = allocator.allocate(buffer_size, |_ptr| {});

        let mut result = SliceObject::new(ptr, new_size, new_capacity);
        result.as_bytes_mut(elem_size).fill(0);

        let lhs_slice = base.as_bytes(elem_size);
        let result_slice = result.as_bytes_mut(elem_size);
        let lhs_len = base.size() * elem_size;
        result_slice[..lhs_len].clone_from_slice(&lhs_slice[..lhs_len]);

        result
    } else {
        base.duplicate_extend(base.size())
    };

    let result_slice = result.as_bytes_mut(elem_size);
    let base_len = base.size() * elem_size;
    let extend_len = extend_bytes.len();
    result_slice[base_len..base_len + extend_len].clone_from_slice(&extend_bytes[..extend_len]);
    result
}

#[repr(C)]
struct StackFrameSliceAppend<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    type_id: TypeId,
    lhs: SliceObject,
    rhs: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceAppend>();
    let lhs = &frame.lhs;
    let rhs = &frame.rhs;

    let elem_size = frame.type_id.size();
    let result = ctx.global_context().process(|mut global_context| {
        reallocate_slice(
            lhs,
            elem_size,
            rhs.as_bytes(elem_size),
            global_context.allocator(),
        )
    });

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppend>();
    *frame.result_ptr = result;

    ctx.leave()
}

#[repr(C)]
struct StackFrameSliceAppendString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    slice: SliceObject,
    string: StringObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_append_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceAppendString>();
    let slice = &frame.slice;
    let string = &frame.string;

    let elem_size = mem::size_of::<u8>();
    let result = ctx.global_context().process(|mut global_context| {
        reallocate_slice(
            slice,
            elem_size,
            string.as_bytes(),
            global_context.allocator(),
        )
    });

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppendString>();
    *frame.result_ptr = result;

    ctx.leave()
}

fn copy_slice(dst: &mut SliceObject, elem_size: usize, src: &[u8]) -> usize {
    assert!(elem_size > 0);
    assert!(src.len() % elem_size == 0);

    let copy_count = cmp::min(src.len() / elem_size, dst.size());
    let src = src.as_ptr();
    let dst = dst.as_bytes_mut(elem_size).as_mut_ptr();
    unsafe {
        ptr::copy(src, dst, elem_size * copy_count);
    }

    copy_count
}

#[repr(C)]
struct StackFrameSliceCopy<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    type_id: TypeId,
    src: SliceObject,
    dst: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_copy(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameSliceCopy>();
    let elem_size = frame.type_id.size();
    let copy_count = copy_slice(&mut frame.dst, elem_size, frame.src.as_bytes(elem_size));
    *frame.result_ptr = isize::try_from(copy_count).unwrap();
    ctx.leave()
}

#[repr(C)]
struct StackFrameSliceCapacity<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    slice: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_capacity(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceCapacity>();
    let result = isize::try_from(frame.slice.capacity()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameSliceCapacity>();
    *frame.result_ptr = result;

    ctx.leave()
}

#[repr(C)]
struct StackFrameSliceSize<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    slice: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_size(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceSize>();
    let result = isize::try_from(frame.slice.size()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameSliceSize>();
    *frame.result_ptr = result;

    ctx.leave()
}
