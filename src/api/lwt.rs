use std::mem;

use crate::create_light_weight_thread_context;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::word_chunk::WordChunk;
use crate::FunctionObject;
use crate::StackFrameCommon;
use crate::UserFunction;

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

#[no_mangle]
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

#[no_mangle]
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

#[no_mangle]
pub extern "C" fn gox5_lwt_yield(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::global_context;
    use crate::ObjectAllocator;
    use crate::UserFunction;
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
