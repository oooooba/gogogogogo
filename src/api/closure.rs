use std::mem;
use std::ptr;

use crate::ClosureLayout;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::word_chunk::WordChunk;

#[repr(C)]
struct StackFrameClosureNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut FunctionObject,
    user_function: UserFunction,
    free_vars: WordChunk,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_closure_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameClosureNew>();

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>(), |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut ClosureLayout);
            }) as *mut ClosureLayout
    });

    let wc_ptr = unsafe {
        let sp = ctx.stack_pointer() as *const u8;
        sp.add(mem::offset_of!(StackFrameClosureNew, free_vars)) as *const WordChunk
    };
    let object_ptrs = unsafe { WordChunk::as_slice_raw::<ObjectPtr>(wc_ptr) }.to_vec();
    let closure_layout = ClosureLayout::new(frame.user_function.clone(), object_ptrs);
    unsafe {
        ptr::write(ptr, closure_layout);
    }

    let frame = ctx.stack_frame_mut::<StackFrameClosureNew>();
    *frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());

    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::UserFunction;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
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

    unsafe extern "C" fn dummy_func(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    #[test]
    fn test_gox5_closure_new() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<FunctionObject>());
        let result_raw = ctx.stack_pointer() as *mut FunctionObject;
        ctx.grow_stack(mem::size_of::<StackFrameClosureNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let wc_buf: [usize; 2] = [0, 0];
        let wc = unsafe { &*(wc_buf.as_ptr() as *const WordChunk) };

        let frame = ctx.stack_frame_mut::<StackFrameClosureNew>();
        frame.user_function = UserFunction::new(dummy_func);
        frame.free_vars = unsafe { ptr::read(wc) };
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_closure_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());

        let res = unsafe { &*result_raw };
        assert_ne!(*res, FunctionObject::new_null());
        let (_uf, obj_ptrs) = res.extract_user_function();
        assert!(obj_ptrs.is_some());
    }

    #[test]
    fn test_gox5_closure_new_with_free_vars() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<FunctionObject>());
        let result_raw = ctx.stack_pointer() as *mut FunctionObject;
        ctx.grow_stack(mem::size_of::<StackFrameClosureNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let dummy_val: isize = 99;
        let dummy_ptr = &dummy_val as *const isize as *const ();
        let wc_buf: [usize; 3] = [1, 0, dummy_ptr as usize];
        let wc = unsafe { &*(wc_buf.as_ptr() as *const WordChunk) };

        let frame = ctx.stack_frame_mut::<StackFrameClosureNew>();
        frame.user_function = UserFunction::new(dummy_func);
        frame.free_vars = unsafe { ptr::read(wc) };
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_closure_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());

        let res = unsafe { &*result_raw };
        assert_ne!(*res, FunctionObject::new_null());
        let (_uf, obj_ptrs) = res.extract_user_function();
        assert!(obj_ptrs.is_some());
    }
}
