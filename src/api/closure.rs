use std::mem;
use std::ptr;

use crate::ClosureLayout;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFrameClosureNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut FunctionObject,
    user_function: UserFunction,
    num_object_ptrs: usize,
    object_ptrs: [ObjectPtr; 0],
}

#[no_mangle]
pub extern "C" fn gox5_closure_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameClosureNew>();

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>(), |_ptr| {}) as *mut ClosureLayout
    });

    let object_ptrs =
        unsafe { std::slice::from_raw_parts(frame.object_ptrs.as_ptr(), frame.num_object_ptrs) }
            .to_vec();
    let closure_layout = ClosureLayout::new(frame.user_function.clone(), object_ptrs);
    unsafe {
        ptr::copy_nonoverlapping(&closure_layout, ptr, 1);
    }
    mem::forget(closure_layout);

    let frame = ctx.stack_frame_mut::<StackFrameClosureNew>();
    *frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());

    ctx.leave()
}
