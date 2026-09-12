use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct Uint32Object {
    raw: u32,
}

#[repr(C)]
struct StackFrameSemaphoreAcquire {
    common: StackFrameCommon,
    s: *mut Uint32Object,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_semaphore_acquire(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSemaphoreAcquire>();
    let s = frame.s;
    unsafe {
        if (*s).raw > 0 {
            (*s).raw -= 1;
            ctx.pop_frame()
        } else {
            ctx.suspend();
            FunctionObject::from_user_function(UserFunction::new(gox5_semaphore_acquire))
        }
    }
}

#[repr(C)]
struct StackFrameSemaphoreRelease {
    common: StackFrameCommon,
    s: *mut Uint32Object,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_semaphore_release(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSemaphoreRelease>();
    unsafe {
        (*frame.s).raw += 1;
    }
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use std::mem;

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let gc = global_context::create_global_context(ObjectAllocator::new());
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_semaphore_acquire_decrements_available_count() {
        let (mut ctx, _gc) = create_ctx();
        let sem = Box::into_raw(Box::new(Uint32Object { raw: 3 }));
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        ctx.grow_stack(mem::size_of::<StackFrameSemaphoreAcquire>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameSemaphoreAcquire>();
            frame.s = sem;
        }

        let result = gox5_semaphore_acquire(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { (*sem).raw }, 2);
        assert!(!ctx.is_suspended());

        unsafe {
            drop(Box::from_raw(sem));
            drop(Box::from_raw(outer_buf));
        }
    }

    #[test]
    fn test_semaphore_acquire_pops_frame_when_count_becomes_zero() {
        let (mut ctx, _gc) = create_ctx();
        let sem = Box::into_raw(Box::new(Uint32Object { raw: 1 }));
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        ctx.grow_stack(mem::size_of::<StackFrameSemaphoreAcquire>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameSemaphoreAcquire>();
            frame.s = sem;
        }

        let result = gox5_semaphore_acquire(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { (*sem).raw }, 0);
        assert!(!ctx.is_suspended());

        unsafe {
            drop(Box::from_raw(sem));
            drop(Box::from_raw(outer_buf));
        }
    }

    #[test]
    fn test_semaphore_acquire_suspends_when_count_is_zero() {
        let (mut ctx, _gc) = create_ctx();
        let sem = Box::into_raw(Box::new(Uint32Object { raw: 0 }));
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        ctx.grow_stack(mem::size_of::<StackFrameSemaphoreAcquire>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameSemaphoreAcquire>();
            frame.s = sem;
        }

        let result = gox5_semaphore_acquire(&mut ctx);
        assert!(ctx.is_suspended());
        assert_eq!(unsafe { (*sem).raw }, 0);
        assert!(result != FunctionObject::new_null());

        // Re-invoking the returned function object re-enters the acquire path
        // and stays suspended while the count remains zero.
        let (user_func, _) = result.extract_user_function();
        let result2 = user_func.invoke(&mut ctx);
        assert!(ctx.is_suspended());
        assert_eq!(unsafe { (*sem).raw }, 0);
        assert!(result2 != FunctionObject::new_null());

        unsafe {
            drop(Box::from_raw(sem));
            drop(Box::from_raw(outer_buf));
        }
    }

    #[test]
    fn test_semaphore_release_increments_count() {
        let (mut ctx, _gc) = create_ctx();
        let sem = Box::into_raw(Box::new(Uint32Object { raw: 0 }));
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        ctx.grow_stack(mem::size_of::<StackFrameSemaphoreRelease>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameSemaphoreRelease>();
            frame.s = sem;
        }

        let result = gox5_semaphore_release(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { (*sem).raw }, 1);
        assert!(!ctx.is_suspended());

        unsafe {
            drop(Box::from_raw(sem));
            drop(Box::from_raw(outer_buf));
        }
    }

    #[test]
    fn test_semaphore_release_increments_count_never_suspends() {
        let (mut ctx, _gc) = create_ctx();
        let sem = Box::into_raw(Box::new(Uint32Object { raw: 5 }));
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        ctx.grow_stack(mem::size_of::<StackFrameSemaphoreRelease>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameSemaphoreRelease>();
            frame.s = sem;
        }

        let result = gox5_semaphore_release(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { (*sem).raw }, 6);

        unsafe {
            drop(Box::from_raw(sem));
            drop(Box::from_raw(outer_buf));
        }
    }
}
