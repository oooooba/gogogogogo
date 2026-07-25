use std::mem;
use std::process;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::object::interface::Interface;

#[repr(C)]
struct StackFramePanicRaise {
    common: StackFrameCommon,
    value: Interface,
}

extern "C" fn panic_raise_body(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let prev_stack_pointer = ctx.stack_pointer();
    let frame = ctx.stack_frame_mut::<StackFramePanicRaise>();
    let prev_frame = frame.common.prev_stack_frame_mut::<StackFrameCommon>();
    match prev_frame.defer_stack_mut().pop() {
        Some(mut entry) => {
            // Keep the stack frame at the time it is called by user function.
            ctx.grow_stack(mem::size_of::<StackFramePanicRaise>());
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
                FunctionObject::from_user_function(UserFunction::new(panic_raise_body)),
            );
            entry.func()
        }
        None => {
            ctx.pop_frame();
            if ctx.is_panicking() {
                if ctx.is_stack_empty() {
                    ctx.exit_panic().panic_print();
                    process::exit(1);
                }
                FunctionObject::from_user_function(UserFunction::new(panic_raise_body))
            } else {
                ctx.pop_frame()
            }
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_panic_raise(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFramePanicRaise>();
    let data = frame.value.clone();
    let data = if data.is_nil() {
        Interface::panic_nil_error()
    } else {
        data
    };
    ctx.enter_panic(data);
    FunctionObject::from_user_function(UserFunction::new(panic_raise_body))
}

#[repr(C)]
struct StackFramePanicRecover<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_panic_recover(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let result = if ctx.is_panicking() {
        ctx.exit_panic()
    } else {
        Interface::nil()
    };
    let frame = ctx.stack_frame_mut::<StackFramePanicRecover>();
    *frame.result_ptr = result;
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
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
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_gox5_panic_recover_not_panicking() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<Interface>());
        let result_raw = ctx.stack_pointer() as *mut Interface;
        ctx.grow_stack(mem::size_of::<StackFramePanicRecover>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFramePanicRecover>();
        frame.result_ptr = unsafe { &mut *result_raw };

        assert!(!ctx.is_panicking());
        let result = gox5_panic_recover(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert!(res.is_nil());
    }

    #[test]
    fn test_gox5_panic_recover_after_panic() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<Interface>());
        let result_raw = ctx.stack_pointer() as *mut Interface;
        ctx.grow_stack(mem::size_of::<StackFramePanicRecover>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let panic_data = Interface::panic_nil_error();
        ctx.enter_panic(panic_data);
        assert!(ctx.is_panicking());

        let frame = ctx.stack_frame_mut::<StackFramePanicRecover>();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_panic_recover(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert!(!ctx.is_panicking());
        let res = unsafe { &*result_raw };
        assert!(res.is_panic_nil_error());
    }

    #[test]
    fn test_gox5_panic_raise() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFramePanicRaise>());
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());

        let frame = ctx.stack_frame_mut::<StackFramePanicRaise>();
        frame.value = Interface::panic_nil_error();

        let result = gox5_panic_raise(&mut ctx);
        assert!(ctx.is_panicking());
        assert_ne!(result, FunctionObject::new_null());
    }

    #[test]
    fn test_gox5_panic_raise_nil() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFramePanicRaise>());
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());

        let frame = ctx.stack_frame_mut::<StackFramePanicRaise>();
        frame.value = Interface::nil();

        let result = gox5_panic_raise(&mut ctx);
        assert!(ctx.is_panicking());
        assert_ne!(result, FunctionObject::new_null());
    }
}
