use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::defer_stack::DeferStackEntry;
use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::word_chunk::WordChunk;

fn register<F>(ctx: &mut LightWeightThreadContext, param: F) -> FunctionObject
where
    F: FnOnce(&LightWeightThreadContext) -> (FunctionObject, usize, *const WordChunk),
{
    let (func, result_size, args) = param(ctx);

    // The copy of the deferred arguments is allocated next to the entry, in a
    // single allocation. It must not be allocated through the global context
    // directly, because a hot loop registering defers can exhaust the heap and
    // the garbage collector has to run before giving up on the allocation. One
    // allocation also keeps the entry and its arguments out of reach of a
    // collection in between: neither is reachable from the stack until the entry
    // is pushed onto the defer stack below.
    let count = unsafe { WordChunk::count_of_raw(args) };
    let entry_size = mem::size_of::<DeferStackEntry>();
    let entry_ptr = ctx.allocate(
        entry_size + WordChunk::size_for_count(count),
        TypeId::new_invalid(),
    ) as *mut DeferStackEntry;
    let args_copy = unsafe { (entry_ptr as *mut u8).add(entry_size) as *mut WordChunk };
    let args = unsafe { WordChunk::copy_into_raw(args_copy, args) };

    let frame = ctx.stack_frame_mut::<StackFrameCommon>();
    let prev_frame = frame.prev_stack_frame_mut::<StackFrameCommon>();

    let entry = DeferStackEntry::new(func, result_size, args);
    unsafe { *entry_ptr = entry };
    let entry_nn = ptr::NonNull::new(entry_ptr).expect("allocator returned null");
    prev_frame.defer_stack_mut().push(entry_nn);

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameDeferRegister {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    args: WordChunk,
}

#[unsafe(no_mangle)]
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

#[unsafe(no_mangle)]
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

#[unsafe(no_mangle)]
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
    use crate::ObjectAllocator;
    use crate::allocator::MAX_TOTAL_ALLOCATED_SIZE;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::type_id::FakeTypeInfo;
    use std::mem;
    use std::ptr;

    unsafe extern "C" fn test_deferred(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    fn total_size(gc: &crate::global_context::GlobalContextPtr) -> usize {
        gc.process(|mut gc| gc.allocator().total_size())
    }

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

    /// The copy of the deferred arguments used to be allocated straight through
    /// the global context, so a hot loop registering defers could exhaust the
    /// heap and get a null pointer back instead of running the garbage
    /// collector.
    #[test]
    fn test_gox5_defer_register_allocates_args_when_heap_is_exhausted() {
        let (mut ctx, gc) = create_ctx();

        // Fill the heap so that exactly one defer stack entry still fits: the
        // entry with its argument copy is larger than that, so it can only be
        // allocated after the garbage collector reclaimed the unreferenced
        // chunk. The fillers are declared pointer-free, which keeps the
        // collection from having to scan this megabyte of them word by word.
        let filler_type = FakeTypeInfo::new(true);
        let collectible_size = 65536;
        let referenced_size =
            MAX_TOTAL_ALLOCATED_SIZE - mem::size_of::<DeferStackEntry>() - collectible_size;

        let (start, _end) = ctx.stack_range();
        ctx.grow_stack(mem::size_of::<usize>());
        let referenced = ctx.allocate(referenced_size, filler_type.tid());
        assert!(!referenced.is_null());
        unsafe { ptr::write(start as *mut usize, referenced as usize) };
        let collectible = ctx.allocate(collectible_size, filler_type.tid());
        assert!(!collectible.is_null());
        let total_before = total_size(&gc);

        // The stack the generated C code builds: a caller frame the deferred
        // call is attached to, and on top of it the register frame holding the
        // argument buffer.
        let func = FunctionObject::from_user_function(UserFunction::new(test_deferred));
        let args: Vec<*const ()> = vec![
            0x1111 as *const (),
            0x2222 as *const (),
            0x3333 as *const (),
            0x4444 as *const (),
        ];
        let prev_stack_pointer = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>());
        ctx.push_frame(prev_stack_pointer, None, &[], FunctionObject::new_null());
        ctx.grow_stack(
            mem::size_of::<StackFrameDeferRegister>()
                + mem::size_of::<WordChunk>()
                + mem::size_of::<*const ()>() * args.len(),
        );
        ctx.push_frame(prev_stack_pointer, None, &[], FunctionObject::new_null());
        let frame = ctx.stack_frame_mut::<StackFrameDeferRegister>();
        frame.func = func.clone();
        frame.result_size = 0;

        // The argument buffer of the register frame: a count followed by the
        // argument words, exactly like the generated C code lays it out. It is
        // written through the raw stack pointer, because
        // `addr_of_mut!(frame.args)` on a `&mut StackFrameDeferRegister` only
        // covers the count word, not the buffer that follows it.
        let mut raw_args: Vec<usize> = Vec::with_capacity(1 + args.len());
        raw_args.push(args.len());
        raw_args.extend(args.iter().map(|arg| *arg as usize));
        unsafe {
            let sp = ctx.stack_pointer() as *mut u8;
            let dst = sp.add(mem::offset_of!(StackFrameDeferRegister, args)) as *mut WordChunk;
            WordChunk::copy_into_raw(dst, raw_args.as_ptr() as *const WordChunk);
        }

        let result = gox5_defer_register(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());

        // The deferred call is registered on the caller frame, with a copy of
        // the arguments that is independent of the register frame's buffer.
        let entry = ctx
            .stack_frame_mut::<StackFrameCommon>()
            .defer_stack_mut()
            .pop()
            .unwrap();
        let entry = unsafe { entry.as_ref() };
        assert_eq!(entry.func(), func);
        assert_eq!(entry.result_size(), 0);
        assert_eq!(entry.args(), args.as_slice());

        assert!(
            total_size(&gc) < total_before,
            "the garbage collector should have reclaimed the unreferenced chunk"
        );
    }
}
