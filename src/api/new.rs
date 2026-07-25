use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut ObjectPtr,
    size: usize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameNew>();
    let size = frame.size;

    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
    unsafe {
        ptr::write_bytes(ptr as *mut u8, 0, size);
    }

    let frame = ctx.stack_frame_mut::<StackFrameNew>();
    *frame.result_ptr = ObjectPtr(ptr);

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
    fn test_gox5_new() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<ObjectPtr>());
        let result_raw = ctx.stack_pointer() as *mut ObjectPtr;
        ctx.grow_stack(mem::size_of::<StackFrameNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameNew>();
        frame.size = 16;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert!(!res.is_null());
        let mem = res.as_ref::<[u8; 16]>();
        assert_eq!(*mem, [0u8; 16]);
    }

    #[test]
    fn test_gox5_new_zero_size() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<ObjectPtr>());
        let result_raw = ctx.stack_pointer() as *mut ObjectPtr;
        ctx.grow_stack(mem::size_of::<StackFrameNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameNew>();
        frame.size = 0;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
    }
}
