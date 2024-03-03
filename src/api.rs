pub(crate) mod channel;
pub(crate) mod defer;
pub(crate) mod map;
pub(crate) mod slice;
pub(crate) mod string;

use std::mem;
use std::ptr;

use super::create_light_weight_thread_context;
use super::interface::Interface;
use super::type_id::TypeId;
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
struct StackFrameMakeInterface {
    common: StackFrameCommon,
    result_ptr: *mut Interface,
    receiver: ObjectPtr,
    type_id: TypeId,
}

pub fn make_interface(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMakeInterface>();

    let receiver = if frame.receiver.is_null() {
        ObjectPtr(ptr::null_mut())
    } else {
        let size = frame.type_id.size();
        let ptr = ctx
            .global_context()
            .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
        unsafe {
            ptr::copy_nonoverlapping(frame.receiver.0 as *const u8, ptr as *mut u8, size);
        }
        ObjectPtr(ptr)
    };

    let interface = Interface::new(receiver, frame.type_id);
    unsafe {
        ptr::copy_nonoverlapping(&interface, frame.result_ptr, 1);
    }
    mem::forget(interface);
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

pub fn schedule(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.leave()
}

#[repr(C)]
struct StackFrameSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [*const (); 0],
}

pub fn spawn(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let new_ctx = {
        let stack_frame = ctx.stack_frame_mut::<StackFrameSpawn>();

        let entry_func = stack_frame.func.clone();
        let result_size = stack_frame.result_size;
        let args = unsafe {
            std::slice::from_raw_parts(
                stack_frame.arg_buffer.as_mut_ptr(),
                stack_frame.num_arg_buffer_words,
            )
        };
        let global_context = ctx.global_context().dupulicate();

        let mut new_ctx = create_light_weight_thread_context(global_context, entry_func);
        new_ctx.call::<StackFrameCommon>(
            result_size,
            args,
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );
        new_ctx
    };
    ctx.global_context.process(|mut global_context| {
        global_context.push_light_weight_thread(new_ctx);
    });
    ctx.suspend();
    ctx.leave()
}
