use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut ObjectPtr,
    size: usize,
}

#[no_mangle]
pub extern "C" fn gox5_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameNew>();
    let size = frame.size;

    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
    unsafe {
        ptr::write_bytes(ptr as *mut u8, 0, size);
    }

    let frame = ctx.stack_frame_mut::<StackFrameNew>();
    *frame.result_ptr = ObjectPtr(ptr);

    ctx.leave()
}
