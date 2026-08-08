use std::mem;
use std::process;

use crate::FunctionObject;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::create_light_weight_thread_context;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::word_chunk::WordChunk;

#[repr(C)]
struct StackFrameLwtExit {
    common: StackFrameCommon,
}

extern "C" fn lwt_exit_body(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let prev_stack_pointer = ctx.stack_pointer();
    let frame = ctx.stack_frame_mut::<StackFrameLwtExit>();
    let prev_frame = frame.common.prev_stack_frame_mut::<StackFrameCommon>();
    match prev_frame.defer_stack_mut().pop() {
        Some(mut entry) => {
            // Keep the stack frame at the time it is called by user function.
            ctx.grow_stack(mem::size_of::<StackFrameLwtExit>());
            let entry = unsafe { entry.as_mut() };
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
                FunctionObject::from_user_function(UserFunction::new(lwt_exit_body)),
            );
            entry.func()
        }
        None => {
            ctx.pop_frame();
            if ctx.is_stack_empty() {
                ctx.suspend();
                ctx.terminate();
                if ctx.is_main() {
                    process::exit(0);
                }
                FunctionObject::new_null()
            } else {
                FunctionObject::from_user_function(UserFunction::new(lwt_exit_body))
            }
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_lwt_exit(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    FunctionObject::from_user_function(UserFunction::new(lwt_exit_body))
}

fn spawn<F>(ctx: &mut LightWeightThreadContext, param: F) -> FunctionObject
where
    F: FnOnce(&LightWeightThreadContext) -> (FunctionObject, usize, *const WordChunk),
{
    let (entry_func, result_size, args) = param(ctx);
    let new_ctx = {
        let global_context = ctx.global_context().dupulicate();
        let mut new_ctx = create_light_weight_thread_context(global_context, entry_func);
        let prev_stack_pointer = new_ctx.stack_pointer();
        let result_pointer = if result_size > 0 {
            Some(prev_stack_pointer as *const ())
        } else {
            None
        };
        new_ctx.grow_stack(result_size);
        new_ctx.push_frame(
            prev_stack_pointer,
            result_pointer,
            unsafe { WordChunk::as_slice_raw(args) },
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );
        new_ctx
    };
    ctx.global_context().process(|mut global_context| {
        global_context.push_light_weight_thread(new_ctx);
    });
    ctx.suspend();
    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameLwtSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    args: WordChunk,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_lwt_spawn(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    spawn(ctx, |ctx| {
        let frame = ctx.stack_frame::<StackFrameLwtSpawn>();
        let entry_func = frame.func.clone();
        let result_size = frame.result_size;
        let args = unsafe {
            let sp = ctx.stack_pointer() as *const u8;
            sp.add(mem::offset_of!(StackFrameLwtSpawn, args)) as *const WordChunk
        };
        (entry_func, result_size, args)
    })
}

#[repr(C)]
struct StackFrameLwtSpawnInvoke<'a> {
    common: StackFrameCommon,
    interface: &'a Interface,
    method_name: StringObject,
    result_size: usize,
    args: WordChunk,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_lwt_spawn_invoke(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    spawn(ctx, |ctx| {
        let frame = ctx.stack_frame::<StackFrameLwtSpawnInvoke>();
        let method = frame.interface.search(frame.method_name.clone());
        let entry_func = method.unwrap();
        let result_size = frame.result_size;
        let args = unsafe {
            let sp = ctx.stack_pointer() as *const u8;
            sp.add(mem::offset_of!(StackFrameLwtSpawnInvoke, args)) as *const WordChunk
        };
        (entry_func, result_size, args)
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_lwt_yield(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::UserFunction;
    use crate::defer_stack::DeferStackEntry;
    use crate::global_context;
    use std::mem;

    struct AllocatedObject {
        ptr: *mut (),
        size: usize,
        destructor: fn(*mut ()),
    }

    struct MockObjectAllocator {
        allocated_objects: Vec<AllocatedObject>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated_objects: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = size.div_ceil(alignment) * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
            self.allocate(num_pages * 4096, |_| {})
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for obj in &self.allocated_objects {
                (obj.destructor)(obj.ptr);
                unsafe {
                    Vec::from_raw_parts(obj.ptr as *mut isize, 0, obj.size);
                }
            }
        }
    }

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let allocator = Box::new(MockObjectAllocator::new());
        let gc = global_context::create_global_context(allocator);
        let func = FunctionObject::new_null();
        let ctx = create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_gox5_lwt_exit() {
        // The first LWT of a fresh global context is the main one (id 0).
        let (_main_ctx, gc) = create_ctx();
        // Create a second LWT so that is_main() is false: calling
        // gox5_lwt_exit must not terminate the process.
        let mut ctx =
            create_light_weight_thread_context(gc.dupulicate(), FunctionObject::new_null());
        assert!(!ctx.is_main());
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<crate::StackFrameCommon>());
        let resume_func = FunctionObject::from_user_function(UserFunction::new(dummy_resume));
        ctx.push_frame(prev_sp, None, &[], resume_func.clone());

        assert!(!ctx.is_terminated());
        // gox5_lwt_exit starts unwinding the stack; the returned function object
        // must be invoked to finish the unwinding and terminate the LWT.
        let result = gox5_lwt_exit(&mut ctx);
        let (exit_body, _) = result.extract_user_function();
        let result = exit_body.invoke(&mut ctx);
        assert!(ctx.is_terminated());
        assert_eq!(result, FunctionObject::new_null());
    }

    static DEFERRED_RAN: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

    unsafe extern "C" fn mark_deferred(ctx: &mut LightWeightThreadContext) -> FunctionObject {
        DEFERRED_RAN.store(true, std::sync::atomic::Ordering::SeqCst);
        let frame = ctx.stack_frame::<StackFrameCommon>();
        frame.resume_func.clone()
    }

    #[test]
    fn test_gox5_lwt_exit_runs_deferred_function() {
        // The first LWT of a fresh global context is the main one (id 0).
        let (_main_ctx, gc) = create_ctx();
        // Create a second LWT so that is_main() is false: calling
        // gox5_lwt_exit must not terminate the process.
        let mut ctx =
            create_light_weight_thread_context(gc.dupulicate(), FunctionObject::new_null());
        assert!(!ctx.is_main());

        // Frame A: the user function frame that registered the deferred function.
        let sp_root = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<crate::StackFrameCommon>());
        ctx.push_frame(
            sp_root,
            None,
            &[],
            FunctionObject::from_user_function(UserFunction::new(dummy_resume)),
        );

        // Register a deferred function on frame A's defer stack.
        let mut args_buf: Vec<usize> = vec![0];
        let entry = Box::into_raw(Box::new(DeferStackEntry::new(
            FunctionObject::from_user_function(UserFunction::new(mark_deferred)),
            0,
            std::ptr::NonNull::new(args_buf.as_mut_ptr() as *mut WordChunk)
                .expect("non-null args pointer"),
        )));
        {
            let frame = ctx.stack_frame_mut::<StackFrameCommon>();
            frame
                .defer_stack_mut()
                .push(std::ptr::NonNull::new(entry).expect("non-null entry pointer"));
        }

        // Frame B: the frame pushed by the Goexit runtime-call instruction in
        // the generated code; gox5_lwt_exit is invoked with this frame on top.
        let sp_a = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameLwtExit>());
        ctx.push_frame(
            sp_a,
            None,
            &[],
            FunctionObject::from_user_function(UserFunction::new(lwt_exit_body)),
        );

        DEFERRED_RAN.store(false, std::sync::atomic::Ordering::SeqCst);
        let mut func = gox5_lwt_exit(&mut ctx);
        while func != FunctionObject::new_null() {
            let (next, _) = func.extract_user_function();
            func = next.invoke(&mut ctx);
        }

        assert!(DEFERRED_RAN.load(std::sync::atomic::Ordering::SeqCst));
        assert!(ctx.is_terminated());
        unsafe {
            drop(Box::from_raw(entry));
        }
    }

    #[test]
    fn test_gox5_lwt_yield() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<crate::StackFrameCommon>());
        let resume_func = FunctionObject::from_user_function(UserFunction::new(dummy_resume));
        ctx.push_frame(prev_sp, None, &[], resume_func.clone());

        assert!(!ctx.is_suspended());
        let result = gox5_lwt_yield(&mut ctx);
        assert!(ctx.is_suspended());
        assert_eq!(result, resume_func);
    }

    #[test]
    fn test_gox5_lwt_yield_pop_frame() {
        let (mut ctx, _gc) = create_ctx();
        let initial_sp = ctx.stack_pointer();

        // Push first frame (like the "main" frame)
        ctx.grow_stack(mem::size_of::<crate::StackFrameCommon>());
        ctx.push_frame(
            initial_sp,
            None,
            &[],
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );

        // Push second frame (the one we'll yield from)
        let sp1 = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<crate::StackFrameCommon>());
        let resume = FunctionObject::from_user_function(UserFunction::new(dummy_resume));
        ctx.push_frame(sp1, None, &[], resume.clone());

        let result = gox5_lwt_yield(&mut ctx);
        assert!(ctx.is_suspended());
        // After pop_frame, should be back at sp1
        assert_eq!(ctx.stack_pointer(), sp1);
        assert_eq!(result, resume);
    }

    unsafe extern "C" fn dummy_resume(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }
}
