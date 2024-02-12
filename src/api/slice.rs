use crate::object::slice::SliceObject;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

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
    let stack_frame = ctx.stack_frame::<StackFrameSliceAppend>();
    let lhs = &stack_frame.lhs;
    let rhs = &stack_frame.rhs;

    let elem_size = stack_frame.type_id.size();

    let new_size = lhs.size() + rhs.size();
    let mut result = if new_size > lhs.capacity() {
        let new_capacity = new_size * 2;
        let buffer_size = new_capacity * elem_size;
        let ptr = ctx.global_context().process(|mut global_context| {
            global_context.allocator().allocate(buffer_size, |_ptr| {})
        });

        let mut result = SliceObject::new(ptr, new_size, new_capacity);
        result.as_bytes_mut(elem_size).fill(0);

        let lhs_slice = lhs.as_bytes(elem_size);
        let result_slice = result.as_bytes_mut(elem_size);
        let lhs_len = lhs.size() * elem_size;
        result_slice[..lhs_len].clone_from_slice(&lhs_slice[..lhs_len]);

        result
    } else {
        lhs.duplicate_extend(lhs.size())
    };

    let rhs_slice = rhs.as_bytes(elem_size);
    let result_slice = result.as_bytes_mut(elem_size);
    let lhs_len = lhs.size() * elem_size;
    let rhs_len = rhs.size() * elem_size;
    result_slice[lhs_len..lhs_len + rhs_len].clone_from_slice(&rhs_slice[..rhs_len]);

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppend>();
    *frame.result_ptr = result;

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
    let stack_frame = ctx.stack_frame::<StackFrameSliceCapacity>();
    let result = isize::try_from(stack_frame.slice.capacity()).unwrap();

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
    let stack_frame = ctx.stack_frame::<StackFrameSliceSize>();
    let result = isize::try_from(stack_frame.slice.size()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameSliceSize>();
    *frame.result_ptr = result;

    ctx.leave()
}
