use std::mem;

use crate::object::slice::SliceObject;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameSliceAppend<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    lhs: SliceObject,
    rhs: SliceObject,
}

#[no_mangle]
pub extern "C" fn gox5_slice_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let stack_frame = ctx.stack_frame::<StackFrameSliceAppend>();
    let lhs = &stack_frame.lhs;
    let rhs = &stack_frame.rhs;

    let new_size = lhs.size() + rhs.size();
    let mut result = if new_size >= lhs.capacity() {
        let new_capacity = new_size * 2;
        let buffer_size = new_capacity * mem::size_of::<isize>();
        let ptr = ctx.global_context().process(|mut global_context| {
            global_context.allocator().allocate(buffer_size, |_ptr| {})
        });

        let mut result = SliceObject::new(ptr, new_size, new_capacity);
        result.as_raw_slice_mut().fill(0);

        let lhs_slice = lhs.as_raw_slice::<isize>();
        let result_slice = result.as_raw_slice_mut();
        result_slice[..lhs_slice.len()].clone_from_slice(&lhs_slice[..lhs_slice.len()]);

        result
    } else {
        lhs.duplicate()
    };

    let rhs_slice = rhs.as_raw_slice::<isize>();
    let result_slice = result.as_raw_slice_mut();
    result_slice[lhs.size()..lhs.size() + rhs_slice.len()].clone_from_slice(rhs_slice);

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppend>();
    *frame.result_ptr = result;

    ctx.leave()
}
