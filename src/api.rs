pub(crate) mod channel;
pub(crate) mod defer;
pub(crate) mod interface;
pub(crate) mod lwt;
pub(crate) mod map;
pub(crate) mod panic;
pub(crate) mod slice;
pub(crate) mod string;

use std::mem;
use std::ptr;

use super::ClosureLayout;
use super::FunctionObject;
use super::LightWeightThreadContext;
use super::ObjectPtr;
use super::StackFrameCommon;
use super::UserFunction;

#[repr(C)]
struct StackFrameMakeClosure {
    common: StackFrameCommon,
    result_ptr: *mut FunctionObject,
    user_function: UserFunction,
    num_object_ptrs: usize,
    object_ptrs: [ObjectPtr; 0],
}

pub fn make_closure(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>(), |_ptr| {})
    });
    unsafe {
        let ptr = ptr as *mut ClosureLayout;
        let closure_layout = &mut *ptr;

        let stack_frame = ctx.stack_frame::<StackFrameMakeClosure>();

        closure_layout.func = stack_frame.user_function.clone();

        let num_object_ptrs = stack_frame.num_object_ptrs;
        let object_ptrs = stack_frame.object_ptrs.as_ptr();
        let object_ptrs = std::slice::from_raw_parts(object_ptrs, num_object_ptrs).to_vec();

        let prev_object_ptrs = mem::replace(&mut closure_layout.object_ptrs, object_ptrs);
        mem::forget(prev_object_ptrs);
    };
    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameMakeClosure>();
        *stack_frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());
    };
    ctx.leave()
}

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
