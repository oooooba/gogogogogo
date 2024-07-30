use std::mem;
use std::ptr;

use crate::defer_stack::DeferStackEntry;
use crate::word_chunk::WordChunk;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;

fn register<F>(ctx: &mut LightWeightThreadContext, param: F) -> FunctionObject
where
    F: FnOnce(&LightWeightThreadContext) -> (FunctionObject, usize, &WordChunk),
{
    let (func, result_size, args) = param(ctx);
    let (args, entry_ptr) = ctx.global_context().process(|mut global_context| {
        let allocator = global_context.allocator();
        let args = args.duplicate(allocator);
        let entry_ptr =
            allocator.allocate(mem::size_of::<DeferStackEntry>(), |_| {}) as *mut DeferStackEntry;
        (args, entry_ptr)
    });

    let frame = ctx.stack_frame_mut::<StackFrameCommon>();
    let prev_frame = frame.prev_stack_frame_mut::<StackFrameCommon>();

    let entry = DeferStackEntry::new(func, result_size, args);
    unsafe {
        *entry_ptr = entry;
        prev_frame
            .defer_stack_mut()
            .push(ptr::NonNull::new_unchecked(entry_ptr));
    }

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameDeferRegister {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    args: WordChunk,
}

#[no_mangle]
pub extern "C" fn gox5_defer_register(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    register(ctx, |ctx| {
        let frame = ctx.stack_frame::<StackFrameDeferRegister>();
        let func = frame.func.clone();
        let result_size = frame.result_size;
        let args = &frame.args;
        (func, result_size, args)
    })
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
        None => return ctx.pop_frame(),
    };

    // Keep the stack frame at the time it is called by user function.
    let prev_stack_pointer = ctx.stack_pointer();
    ctx.grow_stack(mem::size_of::<StackFrameDeferExecute>());

    let result_pointer = if entry.result_size() > 0 {
        Some(ctx.stack_pointer() as *const ())
    } else {
        None
    };
    ctx.grow_stack(entry.result_size());

    ctx.push_frame(
        prev_stack_pointer,
        result_pointer,
        entry.args(),
        FunctionObject::from_user_function(UserFunction::new(gox5_defer_execute)),
    );

    entry.func()
}
