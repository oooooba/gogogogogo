use std::mem;
use std::ptr;

use crate::defer_stack::DeferStackEntry;
use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::word_chunk::WordChunk;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;

fn register<F>(ctx: &mut LightWeightThreadContext, param: F) -> FunctionObject
where
    F: FnOnce(&LightWeightThreadContext) -> (FunctionObject, usize, *const WordChunk),
{
    let (func, result_size, args) = param(ctx);
    let (args, entry_ptr) = ctx.global_context().process(|mut global_context| {
        let allocator = global_context.allocator();
        let args = unsafe { WordChunk::duplicate_raw(args, allocator) };
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
        let args = unsafe {
            let sp = ctx.stack_pointer() as *const u8;
            sp.add(mem::offset_of!(StackFrameDeferRegister, args)) as *const WordChunk
        };
        (func, result_size, args)
    })
}

#[repr(C)]
struct StackFrameDeferRegisterInvoke<'a> {
    common: StackFrameCommon,
    interface: &'a Interface,
    method_name: StringObject,
    result_size: usize,
    args: WordChunk,
}

#[no_mangle]
pub extern "C" fn gox5_defer_register_invoke(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    register(ctx, |ctx| {
        let frame = ctx.stack_frame::<StackFrameDeferRegisterInvoke>();
        let method = frame.interface.search(frame.method_name.clone());
        let func = method.unwrap();
        let result_size = frame.result_size;
        let args = unsafe {
            let sp = ctx.stack_pointer() as *const u8;
            sp.add(mem::offset_of!(StackFrameDeferRegisterInvoke, args)) as *const WordChunk
        };
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::ObjectAllocator;
    use std::mem;
    use std::ptr;

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
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_gox5_defer_register_empty_stack() {
        let (mut ctx, _gc) = create_ctx();
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        // Push inner frame on top of outer frame
        ctx.grow_stack(mem::size_of::<StackFrameDeferRegister>() + mem::size_of::<WordChunk>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        let frame = ctx.stack_frame_mut::<StackFrameDeferRegister>();
        frame.func = FunctionObject::new_null();
        frame.result_size = 0;
        frame.common.free_vars = ptr::null_mut();

        let result = gox5_defer_register(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        unsafe {
            drop(Box::from_raw(outer_buf));
        }
    }

    #[test]
    fn test_gox5_defer_execute_empty_stack() {
        let (mut ctx, _gc) = create_ctx();
        let outer_buf = Box::into_raw(Box::new([0usize; 128]));
        let outer_sp = outer_buf as *mut crate::StackFrame;

        // Push defer_execute frame on top of outer frame
        ctx.grow_stack(mem::size_of::<StackFrameDeferExecute>());
        ctx.push_frame(outer_sp, None, &[], FunctionObject::new_null());

        // Outer frame has empty defer stack, so defer_execute should just pop
        let result = gox5_defer_execute(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        unsafe {
            drop(Box::from_raw(outer_buf));
        }
    }
}
