use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::create_light_weight_thread_context;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::type_id::TypeId;

#[repr(C)]
pub(crate) struct CoroObject {
    slot: usize,
}

#[repr(C)]
struct StackFrameCoroNew {
    common: StackFrameCommon,
    result_ptr: *mut *mut CoroObject,
    function_object: FunctionObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_coro_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameCoroNew>();
    let entry_func = frame.function_object.clone();
    let result_ptr = frame.result_ptr;

    let global_context = ctx.global_context().dupulicate();
    let slot = ctx
        .global_context()
        .process(|mut gc| gc.reserve_coro_slot());
    let coro = ctx.global_context().process(|mut gc| {
        gc.allocator()
            .allocate(mem::size_of::<CoroObject>(), TypeId::new_invalid())
    });
    unsafe {
        ptr::write(coro as *mut CoroObject, CoroObject { slot });
    }

    let mut new_ctx = create_light_weight_thread_context(global_context, entry_func);
    let prev_stack_pointer = new_ctx.stack_pointer();
    new_ctx.push_frame(
        prev_stack_pointer,
        None,
        &[coro as *const ()],
        FunctionObject::from_user_function(UserFunction::new(gox5_coro_switch)),
    );
    ctx.global_context().process(|mut gc| {
        gc.park_coro(slot, new_ctx);
    });

    unsafe {
        ptr::write(result_ptr, coro as *mut CoroObject);
    }
    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameCoroSwitch {
    common: StackFrameCommon,
    coro: *mut CoroObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_coro_switch(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameCoroSwitch>();
    let slot = unsafe { (*frame.coro).slot };
    ctx.global_context().process(|mut gc| {
        let parked = gc.unpark_coro(slot);
        gc.push_light_weight_thread(parked);
    });
    if ctx.is_stack_empty() {
        ctx.suspend();
        ctx.terminate();
        return FunctionObject::new_null();
    }
    ctx.set_coro_slot(Some(slot));
    ctx.suspend();
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let gc = global_context::create_global_context(ObjectAllocator::new());
        let func = FunctionObject::new_null();
        let ctx = create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    unsafe extern "C" fn dummy_resume(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    #[test]
    fn test_coro_slot_reserve_park_unpark() {
        let (mut ctx, gc) = create_ctx();
        let slot = gc.process(|mut gc| gc.reserve_coro_slot());
        assert_eq!(slot, 0);

        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>());
        let resume_func = FunctionObject::from_user_function(UserFunction::new(dummy_resume));
        ctx.push_frame(prev_sp, None, &[], resume_func);

        gc.process(|mut gc| {
            gc.park_coro(slot, ctx);
        });

        gc.process(|mut gc| {
            let parked = gc.unpark_coro(slot);
            assert!(!parked.is_terminated());
        });
    }
}
