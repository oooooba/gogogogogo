pub(crate) mod channel;
pub(crate) mod map;
pub(crate) mod slice;
pub(crate) mod string;

use std::mem;
use std::ptr;

use crate::gox5_run_defers;

use super::create_light_weight_thread_context;
use super::interface::Interface;
use super::type_id::TypeId;
use super::ClosureLayout;
use super::FunctionObject;
use super::LightWeightThreadContext;
use super::ObjectPtr;
use super::StackFrame;
use super::StackFrameCommon;
use super::UserFunction;

struct Deferred {
    prev_deferred: *const (),
    target_stack_pointer: *const (),
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: *const *const (),
}

#[repr(C)]
struct StackFrameDefer {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [(); 0],
}

pub fn defer(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (func, result_size, num_arg_buffer_words) = {
        let stack_frame = ctx.stack_frame::<StackFrameDefer>();
        (
            stack_frame.func.clone(),
            stack_frame.result_size,
            stack_frame.num_arg_buffer_words,
        )
    };

    let dst_arg_buffer = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<*const ()>() * num_arg_buffer_words, |_| {})
            as *mut *const ()
    });

    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameDefer>();
        let src = stack_frame.arg_buffer.as_ptr() as *const *const ();
        ptr::copy_nonoverlapping(src, dst_arg_buffer, num_arg_buffer_words);
    }

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<Deferred>(), |_| {}) as *mut Deferred
    });
    let prev_deferred = ctx.deferred_list();
    let deferred = Deferred {
        prev_deferred,
        target_stack_pointer: ctx.stack_pointer(),
        func,
        result_size,
        num_arg_buffer_words,
        arg_buffer: dst_arg_buffer,
    };
    unsafe {
        ptr::copy_nonoverlapping(&deferred, ptr, 1);
    }
    mem::forget(deferred);

    ctx.update_deferred_list(ptr as *const ());

    ctx.leave()
}

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
    ctx.request_suspend();
    ctx.leave()
}

#[repr(C)]
struct StackFrameRunDefers {
    common: StackFrameCommon,
}

pub fn run_defers(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    if ctx.deferred_list().is_null() {
        return ctx.leave();
    }

    let deferred = unsafe { &*(ctx.deferred_list() as *const Deferred) };

    if deferred.target_stack_pointer != ctx.stack_pointer() {
        return ctx.leave();
    }

    ctx.update_deferred_list(deferred.prev_deferred);

    let current_stack_pointer = ctx.stack_pointer() as *mut StackFrame;
    let next_stack_pointer = unsafe { (current_stack_pointer as *mut StackFrameRunDefers).add(1) };
    let result_pointer = next_stack_pointer as *mut *const ();
    let next_stack_pointer = unsafe { (next_stack_pointer as *mut u8).add(deferred.result_size) };
    let next_stack_pointer = next_stack_pointer as *mut StackFrameCommon;
    let next_frame = unsafe { &mut (*next_stack_pointer) };
    next_frame.resume_func = FunctionObject::from_user_function(UserFunction::new(gox5_run_defers));
    next_frame.prev_stack_pointer = current_stack_pointer;
    next_frame.free_vars = ptr::null_mut();

    unsafe {
        let src = deferred.arg_buffer;
        let dst = (next_stack_pointer).add(1);
        let dst = dst as *mut *const ();
        let dst = if deferred.result_size > 0 {
            *(dst as *mut *mut *const ()) = result_pointer;
            dst.add(1)
        } else {
            dst
        };
        ptr::copy_nonoverlapping(src, dst, deferred.num_arg_buffer_words);
    }

    ctx.update_stack_pointer(next_stack_pointer as *mut StackFrame);

    deferred.func.clone()
}

#[repr(C)]
struct StackFrameSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [(); 0],
}

pub fn spawn(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let new_ctx = {
        let stack_frame = ctx.stack_frame_mut::<StackFrameSpawn>();

        let entry_func = stack_frame.func.clone();
        let result_size = stack_frame.result_size;
        let arg_buffer_ptr = ObjectPtr(stack_frame.arg_buffer.as_mut_ptr());
        let num_arg_buffer_words = stack_frame.num_arg_buffer_words;
        let global_context = ctx.global_context().dupulicate();

        let mut new_ctx = create_light_weight_thread_context(global_context);
        new_ctx.set_up(
            entry_func,
            result_size,
            arg_buffer_ptr,
            num_arg_buffer_words,
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );
        new_ctx
    };
    ctx.global_context.process(|mut global_context| {
        global_context.push_light_weight_thread(new_ctx);
    });
    ctx.request_suspend();
    ctx.leave()
}
