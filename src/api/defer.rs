use core::slice;
use std::mem;
use std::ptr;

use crate::defer_stack::DeferStackEntry;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFrameDeferRegister {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [*const (); 0],
}

#[no_mangle]
pub extern "C" fn gox5_defer_register(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameDeferRegister>();

    let dst_arg_buffer = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(
            mem::size_of::<*const ()>() * frame.num_arg_buffer_words,
            |_| {},
        ) as *mut *const ()
    });

    let entry_ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<DeferStackEntry>(), |_| {}) as *mut DeferStackEntry
    });

    unsafe {
        let src = slice::from_raw_parts(frame.arg_buffer.as_ptr(), frame.num_arg_buffer_words);
        let dst = slice::from_raw_parts_mut(dst_arg_buffer, frame.num_arg_buffer_words);
        dst.copy_from_slice(src);
    }

    let frame = ctx.stack_frame_mut::<StackFrameDeferRegister>();
    let prev_frame = frame.common.prev_stack_frame_mut::<StackFrameCommon>();

    let entry = DeferStackEntry::new(
        frame.func.clone(),
        frame.result_size,
        frame.num_arg_buffer_words,
        dst_arg_buffer,
    );
    unsafe {
        *entry_ptr = entry;
        prev_frame
            .defer_stack_mut()
            .push(ptr::NonNull::new_unchecked(entry_ptr));
    }

    ctx.leave()
}

#[repr(C)]
struct StackFrameDeferExecute {
    common: StackFrameCommon,
}

#[no_mangle]
pub extern "C" fn gox5_defer_execute(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameDeferExecute>();
    let prev_frame = frame.common.prev_stack_frame_mut::<StackFrameCommon>();

    let entry = match prev_frame.defer_stack_mut().pop() {
        Some(mut entry) => unsafe { entry.as_mut() },
        None => return ctx.leave(),
    };

    ctx.call::<StackFrameDeferExecute>(
        entry.result_size(),
        entry.args(),
        FunctionObject::from_user_function(UserFunction::new(gox5_defer_execute)),
    );

    entry.func()
}
