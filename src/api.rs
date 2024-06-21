pub(crate) mod channel;
pub(crate) mod closure;
pub(crate) mod defer;
pub(crate) mod interface;
pub(crate) mod lwt;
pub(crate) mod map;
pub(crate) mod panic;
pub(crate) mod slice;
pub(crate) mod string;

use std::ptr;

use super::FunctionObject;
use super::LightWeightThreadContext;
use super::ObjectPtr;
use super::StackFrameCommon;

#[repr(C)]
struct StackFrameNew {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

pub fn new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let size = {
        let stack_frame = ctx.stack_frame::<StackFrameNew>();
        stack_frame.size
    };
    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
    unsafe {
        ptr::write_bytes(ptr as *mut u8, 0, size);
    }
    let ptr = ObjectPtr(ptr);
    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameNew>();
        *stack_frame.result_ptr = ptr;
    };
    ctx.leave()
}
