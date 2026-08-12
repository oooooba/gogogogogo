use std::cmp;
use std::iter;
use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectAllocator;
use crate::StackFrameCommon;
use crate::object::slice::SliceObject;
use crate::object::string::StringObject;
use crate::type_id::TypeId;

#[repr(C)]
struct StackFrameSliceFromString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    type_id: TypeId,
    src: StringObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_from_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceFromString>();

    let elem_size = frame.type_id.size();
    assert!(elem_size == mem::size_of::<u8>() || elem_size == mem::size_of::<u32>());

    let len = frame.src.len_in_bytes();
    let buffer_size = len * elem_size;
    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(buffer_size, |_ptr| {}));

    let mut result = SliceObject::new(ptr, len, len);
    if frame.type_id.size() == mem::size_of::<u8>() {
        result
            .as_bytes_mut(elem_size)
            .clone_from_slice(frame.src.as_bytes());
    } else {
        let s = frame.src.to_str().unwrap();
        iter::zip(
            result.as_bytes_mut(elem_size)[..buffer_size].chunks_mut(elem_size),
            s.chars(),
        )
        .for_each(|(dst_bytes, ch)| {
            let src_bytes = (ch as u32).to_le_bytes();
            dst_bytes.clone_from_slice(&src_bytes);
        });
    }

    let frame = ctx.stack_frame_mut::<StackFrameSliceFromString>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

fn slice_extend_bytes(slice: &SliceObject, elem_size: usize) -> &[u8] {
    &slice.as_bytes(elem_size)[..slice.size() * elem_size]
}

fn reallocate_slice(
    base: &SliceObject,
    elem_size: usize,
    extend_bytes: &[u8],
    allocator: &mut dyn ObjectAllocator,
) -> SliceObject {
    assert!(elem_size > 0);
    assert!(extend_bytes.len().is_multiple_of(elem_size));

    let new_size = base.size() + extend_bytes.len() / elem_size;
    let mut result = if new_size > base.capacity() {
        let new_capacity = new_size * 2;
        let buffer_size = new_capacity * elem_size;
        let ptr = allocator.allocate(buffer_size, |_ptr| {});

        let mut result = SliceObject::new(ptr, new_size, new_capacity);
        result.as_bytes_mut(elem_size).fill(0);

        let lhs_slice = base.as_bytes(elem_size);
        let result_slice = result.as_bytes_mut(elem_size);
        let lhs_len = base.size() * elem_size;
        result_slice[..lhs_len].clone_from_slice(&lhs_slice[..lhs_len]);

        result
    } else {
        base.duplicate_extend(extend_bytes.len() / elem_size)
    };

    let result_slice = result.as_bytes_mut(elem_size);
    let base_len = base.size() * elem_size;
    let extend_len = extend_bytes.len();
    let src = extend_bytes.as_ptr();
    let dst = unsafe { result_slice.as_mut_ptr().add(base_len) };
    unsafe {
        let src = core::hint::black_box(src);
        let dst = core::hint::black_box(dst);
        ptr::copy(src, dst, extend_len);
    }
    result
}

#[repr(C)]
struct StackFrameSliceAppend<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    type_id: TypeId,
    lhs: SliceObject,
    rhs: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceAppend>();
    let lhs = &frame.lhs;
    let rhs = &frame.rhs;

    let elem_size = frame.type_id.size();
    let rhs_bytes = slice_extend_bytes(rhs, elem_size);
    let result = ctx.global_context().process(|mut global_context| {
        reallocate_slice(lhs, elem_size, rhs_bytes, global_context.allocator())
    });

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppend>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceAppendString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    slice: SliceObject,
    string: StringObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_append_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceAppendString>();
    let slice = &frame.slice;
    let string = &frame.string;

    let elem_size = mem::size_of::<u8>();
    let result = ctx.global_context().process(|mut global_context| {
        reallocate_slice(
            slice,
            elem_size,
            string.as_bytes(),
            global_context.allocator(),
        )
    });

    let frame = ctx.stack_frame_mut::<StackFrameSliceAppendString>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

fn copy_slice(dst: &mut SliceObject, elem_size: usize, src: &[u8], src_size: usize) -> usize {
    assert!(elem_size > 0);
    assert!(src.len().is_multiple_of(elem_size));

    let copy_count = cmp::min(src_size, dst.size());
    let src = src.as_ptr();
    let dst = dst.as_bytes_mut(elem_size).as_mut_ptr();
    unsafe {
        let src = core::hint::black_box(src);
        let dst = core::hint::black_box(dst);
        ptr::copy(src, dst, elem_size * copy_count);
    }

    copy_count
}

#[repr(C)]
struct StackFrameSliceCopy<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    type_id: TypeId,
    src: SliceObject,
    dst: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_copy(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameSliceCopy>();
    let elem_size = frame.type_id.size();
    let copy_count = copy_slice(
        &mut frame.dst,
        elem_size,
        frame.src.as_bytes(elem_size),
        frame.src.size(),
    );
    *frame.result_ptr = isize::try_from(copy_count).unwrap();
    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceCopyString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    src: StringObject,
    dst: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_copy_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameSliceCopyString>();
    let elem_size = mem::size_of::<u8>();
    let copy_count = copy_slice(
        &mut frame.dst,
        elem_size,
        frame.src.as_bytes(),
        frame.src.len_in_bytes(),
    );
    *frame.result_ptr = isize::try_from(copy_count).unwrap();
    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceCapacity<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    slice: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_capacity(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceCapacity>();
    let result = isize::try_from(frame.slice.capacity()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameSliceCapacity>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceSize<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    slice: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_size(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceSize>();
    let result = isize::try_from(frame.slice.size()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameSliceSize>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

fn slice_bytes(slice: &SliceObject) -> &[u8] {
    &slice.as_bytes(mem::size_of::<u8>())[..slice.size()]
}

fn index_of(s: &[u8], sep: &[u8]) -> isize {
    if sep.is_empty() {
        return 0;
    }
    if sep.len() > s.len() {
        return -1;
    }
    s.windows(sep.len())
        .position(|window| window == sep)
        .map_or(-1, |index| isize::try_from(index).unwrap())
}

#[repr(C)]
struct StackFrameSliceCompare<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    lhs: SliceObject,
    rhs: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_compare(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceCompare>();
    let result = match slice_bytes(&frame.lhs).cmp(slice_bytes(&frame.rhs)) {
        cmp::Ordering::Less => -1,
        cmp::Ordering::Equal => 0,
        cmp::Ordering::Greater => 1,
    };

    let frame = ctx.stack_frame_mut::<StackFrameSliceCompare>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceCount<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    b: SliceObject,
    c: u8,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_count(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceCount>();
    let result = slice_bytes(&frame.b)
        .iter()
        .filter(|&&x| x == frame.c)
        .count() as isize;

    let frame = ctx.stack_frame_mut::<StackFrameSliceCount>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceSearchSlice<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    lhs: SliceObject,
    rhs: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_search_slice(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceSearchSlice>();
    let result = index_of(slice_bytes(&frame.lhs), slice_bytes(&frame.rhs));

    let frame = ctx.stack_frame_mut::<StackFrameSliceSearchSlice>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceSearchByte<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    b: SliceObject,
    c: u8,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_search_byte(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceSearchByte>();
    let result = slice_bytes(&frame.b)
        .iter()
        .position(|&x| x == frame.c)
        .map_or(-1, |i| isize::try_from(i).unwrap());

    let frame = ctx.stack_frame_mut::<StackFrameSliceSearchByte>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameSliceNewUninitialized<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut SliceObject,
    n: isize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_slice_new_uninitialized(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameSliceNewUninitialized>();
    let n = usize::try_from(frame.n).unwrap();

    let result = if n == 0 {
        SliceObject::new(ptr::null_mut(), 0, 0)
    } else {
        let ptr = ctx
            .global_context()
            .process(|mut global_context| global_context.allocator().allocate(n, |_ptr| {}));
        SliceObject::new(ptr, n, n)
    };

    let frame = ctx.stack_frame_mut::<StackFrameSliceNewUninitialized>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
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

    #[test]
    fn test_slice_extend_bytes_clamps_to_size() {
        let mut buf = [0u8; 64];
        buf[0] = 10;
        buf[1] = 20;
        buf[2] = 30;
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 2, 10);
        let bytes = slice_extend_bytes(&slice, 1);
        assert_eq!(bytes.len(), 2);
        assert_eq!(bytes[0], 10);
        assert_eq!(bytes[1], 20);
    }

    #[test]
    fn test_reallocate_slice_within_capacity() {
        let mut allocator = MockObjectAllocator::new();
        let mut buf = [0u8; 64];
        buf[0] = 10;
        buf[1] = 20;
        buf[2] = 30;
        let ptr = buf.as_mut_ptr() as *mut ();
        let base = SliceObject::new(ptr, 3, 10);
        let extend = [40u8, 50];
        let result = reallocate_slice(&base, 1, &extend, &mut allocator);
        assert_eq!(result.size(), 5);
        assert_eq!(result.capacity(), 10);
        let bytes = result.as_bytes(1);
        assert_eq!(bytes[0], 10);
        assert_eq!(bytes[1], 20);
        assert_eq!(bytes[2], 30);
        assert_eq!(bytes[3], 40);
        assert_eq!(bytes[4], 50);
    }

    #[test]
    fn test_reallocate_slice_overflow_triggers_realloc() {
        let mut allocator = MockObjectAllocator::new();
        let mut buf = [0u8; 4];
        buf[0] = 1;
        buf[1] = 2;
        let ptr = buf.as_mut_ptr() as *mut ();
        let base = SliceObject::new(ptr, 2, 2);
        let extend = [3u8, 4, 5, 6];
        let result = reallocate_slice(&base, 1, &extend, &mut allocator);
        assert_eq!(result.size(), 6);
        assert_eq!(result.capacity(), 12);
        let bytes = result.as_bytes(1);
        assert_eq!(bytes[0], 1);
        assert_eq!(bytes[1], 2);
        assert_eq!(bytes[2], 3);
        assert_eq!(bytes[3], 4);
        assert_eq!(bytes[4], 5);
        assert_eq!(bytes[5], 6);
    }

    #[test]
    fn test_reallocate_slice_u32_elements() {
        let mut allocator = MockObjectAllocator::new();
        let mut buf = [0u32; 4];
        buf[0] = 100;
        buf[1] = 200;
        let ptr = buf.as_mut_ptr() as *mut ();
        let base = SliceObject::new(ptr, 2, 4);
        let extend = 300u32.to_le_bytes();
        let extend2 = 400u32.to_le_bytes();
        let mut extend_bytes = Vec::new();
        extend_bytes.extend_from_slice(&extend);
        extend_bytes.extend_from_slice(&extend2);
        let result = reallocate_slice(&base, 4, &extend_bytes, &mut allocator);
        assert_eq!(result.size(), 4);
        assert_eq!(result.capacity(), 4);
    }

    #[test]
    fn test_copy_slice_normal() {
        let mut dst_buf = [0u8; 8];
        let mut dst_slice = SliceObject::new(dst_buf.as_mut_ptr() as *mut (), 4, 8);
        let src = [10u8, 20, 30, 40, 50, 60];
        let count = copy_slice(&mut dst_slice, 1, &src, src.len());
        assert_eq!(count, 4);
        assert_eq!(dst_buf[0], 10);
        assert_eq!(dst_buf[1], 20);
        assert_eq!(dst_buf[2], 30);
        assert_eq!(dst_buf[3], 40);
        assert_eq!(dst_buf[4], 0);
    }

    #[test]
    fn test_copy_slice_src_larger_than_dst() {
        let mut dst_buf = [0u8; 3];
        let mut dst_slice = SliceObject::new(dst_buf.as_mut_ptr() as *mut (), 3, 3);
        let src = [10u8, 20, 30, 40, 50];
        let count = copy_slice(&mut dst_slice, 1, &src, src.len());
        assert_eq!(count, 3);
        assert_eq!(dst_buf[0], 10);
        assert_eq!(dst_buf[1], 20);
        assert_eq!(dst_buf[2], 30);
    }

    #[test]
    fn test_copy_slice_dst_larger_than_src() {
        let mut dst_buf = [0u8; 8];
        let mut dst_slice = SliceObject::new(dst_buf.as_mut_ptr() as *mut (), 8, 8);
        let src = [10u8, 20];
        let count = copy_slice(&mut dst_slice, 1, &src, src.len());
        assert_eq!(count, 2);
        assert_eq!(dst_buf[0], 10);
        assert_eq!(dst_buf[1], 20);
        assert_eq!(dst_buf[2], 0);
    }

    #[test]
    fn test_copy_slice_u32_elements() {
        let mut dst_buf = [0u32; 4];
        let mut dst_slice = SliceObject::new(dst_buf.as_mut_ptr() as *mut (), 2, 4);
        let src = [100u32, 200, 300];
        let mut src_bytes = Vec::new();
        for v in &src {
            src_bytes.extend_from_slice(&v.to_le_bytes());
        }
        let count = copy_slice(&mut dst_slice, 4, &src_bytes, src.len());
        assert_eq!(count, 2);
        assert_eq!(dst_buf[0], 100);
        assert_eq!(dst_buf[1], 200);
        assert_eq!(dst_buf[2], 0);
    }

    // --- gox5 function tests ---

    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;

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
    fn test_gox5_slice_size() {
        let (mut ctx, _gc) = create_ctx();
        let mut buf = [0u8; 64];
        buf[0] = 1;
        buf[1] = 2;
        buf[2] = 3;
        let slice_obj = SliceObject::new(buf.as_mut_ptr() as *mut (), 3, 10);

        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceSize>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceSize>();
        frame.slice = slice_obj;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_slice_size(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 3);
    }

    #[test]
    fn test_gox5_slice_capacity() {
        let (mut ctx, _gc) = create_ctx();
        let mut buf = [0u8; 64];
        let slice_obj = SliceObject::new(buf.as_mut_ptr() as *mut (), 3, 10);

        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceCapacity>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceCapacity>();
        frame.slice = slice_obj;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_slice_capacity(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 10);
    }

    fn raw_slice(bytes: &'static [u8]) -> SliceObject {
        SliceObject::new(bytes.as_ptr() as *mut (), bytes.len(), bytes.len())
    }

    fn call_slice_compare(
        ctx: &mut LightWeightThreadContext,
        lhs: SliceObject,
        rhs: SliceObject,
    ) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceCompare>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceCompare>();
        frame.lhs = lhs;
        frame.rhs = rhs;
        frame.result_ptr = unsafe { &mut *result_raw };

        assert_eq!(gox5_slice_compare(ctx), FunctionObject::new_null());
        unsafe { *result_raw }
    }

    fn call_slice_count(ctx: &mut LightWeightThreadContext, b: SliceObject, c: u8) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceCount>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceCount>();
        frame.b = b;
        frame.c = c;
        frame.result_ptr = unsafe { &mut *result_raw };

        assert_eq!(gox5_slice_count(ctx), FunctionObject::new_null());
        unsafe { *result_raw }
    }

    fn call_slice_search_slice(
        ctx: &mut LightWeightThreadContext,
        lhs: SliceObject,
        rhs: SliceObject,
    ) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceSearchSlice>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceSearchSlice>();
        frame.lhs = lhs;
        frame.rhs = rhs;
        frame.result_ptr = unsafe { &mut *result_raw };

        assert_eq!(gox5_slice_search_slice(ctx), FunctionObject::new_null());
        unsafe { *result_raw }
    }

    fn call_slice_search_byte(ctx: &mut LightWeightThreadContext, b: SliceObject, c: u8) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameSliceSearchByte>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceSearchByte>();
        frame.b = b;
        frame.c = c;
        frame.result_ptr = unsafe { &mut *result_raw };

        assert_eq!(gox5_slice_search_byte(ctx), FunctionObject::new_null());
        unsafe { *result_raw }
    }

    fn call_slice_new_uninitialized(ctx: &mut LightWeightThreadContext, n: isize) -> SliceObject {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<SliceObject>());
        let result_raw = ctx.stack_pointer() as *mut SliceObject;
        ctx.grow_stack(mem::size_of::<StackFrameSliceNewUninitialized>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameSliceNewUninitialized>();
        frame.n = n;
        frame.result_ptr = unsafe { &mut *result_raw };

        assert_eq!(
            gox5_slice_new_uninitialized(ctx),
            FunctionObject::new_null()
        );
        unsafe { (result_raw as *const SliceObject).read() }
    }

    #[test]
    fn test_gox5_slice_compare() {
        let (mut ctx, _gc) = create_ctx();
        assert_eq!(
            call_slice_compare(&mut ctx, raw_slice(b"abc"), raw_slice(b"abd")),
            -1
        );
        assert_eq!(
            call_slice_compare(&mut ctx, raw_slice(b"abd"), raw_slice(b"abc")),
            1
        );
        assert_eq!(
            call_slice_compare(&mut ctx, raw_slice(b"abc"), raw_slice(b"abc")),
            0
        );
        assert_eq!(
            call_slice_compare(&mut ctx, raw_slice(b""), raw_slice(b"abc")),
            -1
        );
        assert_eq!(
            call_slice_compare(&mut ctx, raw_slice(b"abc"), raw_slice(b"ab")),
            1
        );
    }

    #[test]
    fn test_gox5_slice_count() {
        let (mut ctx, _gc) = create_ctx();
        assert_eq!(call_slice_count(&mut ctx, raw_slice(b"abacada"), b'a'), 4);
        assert_eq!(call_slice_count(&mut ctx, raw_slice(b"abacada"), b'x'), 0);
        assert_eq!(call_slice_count(&mut ctx, raw_slice(b""), b'a'), 0);
    }

    #[test]
    fn test_gox5_slice_search_slice() {
        let (mut ctx, _gc) = create_ctx();
        assert_eq!(
            call_slice_search_slice(&mut ctx, raw_slice(b"hello world"), raw_slice(b"lo wo")),
            3
        );
        assert_eq!(
            call_slice_search_slice(&mut ctx, raw_slice(b"hello"), raw_slice(b"xyz")),
            -1
        );
        assert_eq!(
            call_slice_search_slice(&mut ctx, raw_slice(b"hello"), raw_slice(b"")),
            0
        );
        assert_eq!(
            call_slice_search_slice(&mut ctx, raw_slice(b""), raw_slice(b"hello")),
            -1
        );
        assert_eq!(
            call_slice_search_slice(&mut ctx, raw_slice(b"abab"), raw_slice(b"ba")),
            1
        );
    }

    #[test]
    fn test_gox5_slice_search_byte() {
        let (mut ctx, _gc) = create_ctx();
        assert_eq!(
            call_slice_search_byte(&mut ctx, raw_slice(b"hello"), b'l'),
            2
        );
        assert_eq!(
            call_slice_search_byte(&mut ctx, raw_slice(b"hello"), b'h'),
            0
        );
        assert_eq!(
            call_slice_search_byte(&mut ctx, raw_slice(b"hello"), b'x'),
            -1
        );
        assert_eq!(call_slice_search_byte(&mut ctx, raw_slice(b""), b'a'), -1);
    }

    #[test]
    fn test_gox5_slice_new_uninitialized() {
        let (mut ctx, _gc) = create_ctx();
        let s = call_slice_new_uninitialized(&mut ctx, 5);
        assert_eq!(s.size(), 5);
        assert_eq!(s.capacity(), 5);
        let bytes = s.as_bytes(mem::size_of::<u8>());
        assert_eq!(&bytes[..5], &[0u8; 5]);

        let s = call_slice_new_uninitialized(&mut ctx, 16);
        assert_eq!(s.size(), 16);
        assert_eq!(s.capacity(), 16);

        let s = call_slice_new_uninitialized(&mut ctx, 0);
        assert_eq!(s.size(), 0);
        assert_eq!(s.capacity(), 0);
    }
}
