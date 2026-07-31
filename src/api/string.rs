use std::mem;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::object::slice::SliceObject;
use crate::object::string::StringObject;

#[repr(C)]
struct StackFrameStringNewFromByteSlice<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    byte_slice: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_new_from_byte_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringNewFromByteSlice>();
    let len = frame.byte_slice.size();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    let src_bytes = frame.byte_slice.as_bytes(mem::size_of::<u8>());
    builder.append_bytes(&src_bytes[..len]);

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromByteSlice>();
    *frame.result_ptr = builder.build();

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringNewFromRune<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    rune: usize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_new_from_rune(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringNewFromRune>();
    let rune = frame.rune;

    assert!(rune <= u32::MAX as usize);
    let ch = char::from_u32(rune as u32).unwrap();
    let len = ch.len_utf8();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_char(ch);

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRune>();
    *frame.result_ptr = builder.build();

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringNewFromRuneSlice<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    rune_slice: SliceObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_new_from_rune_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let stack_frame = ctx.stack_frame::<StackFrameStringNewFromRuneSlice>();
    let rune_slice = &stack_frame.rune_slice;

    let len = {
        let elem_size = mem::size_of::<u32>();
        let src_bytes = rune_slice.as_bytes(elem_size);
        src_bytes[..rune_slice.size() * elem_size]
            .chunks(elem_size)
            .fold(0, |acc, bytes| {
                let rune = u32::from_le_bytes(bytes.try_into().unwrap());
                let ch = char::from_u32(rune).unwrap();
                acc + ch.len_utf8()
            })
    };

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    let elem_size = mem::size_of::<u32>();
    let src_bytes = rune_slice.as_bytes(elem_size);
    src_bytes[..rune_slice.size() * elem_size]
        .chunks(elem_size)
        .for_each(|bytes| {
            let rune = u32::from_le_bytes(bytes.try_into().unwrap());
            let ch = char::from_u32(rune).unwrap();
            builder.append_char(ch);
        });

    let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRuneSlice>();
    *frame.result_ptr = builder.build();

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringAppend<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    lhs: StringObject,
    rhs: StringObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringAppend>();
    let len = frame.lhs.len_in_bytes() + frame.rhs.len_in_bytes();

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_bytes(frame.lhs.as_bytes());
    builder.append_bytes(frame.rhs.as_bytes());

    let frame = ctx.stack_frame_mut::<StackFrameStringAppend>();
    *frame.result_ptr = builder.build();

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringLength<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    string: StringObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_length(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringLength>();
    let result = isize::try_from(frame.string.len_in_bytes()).unwrap();

    let frame = ctx.stack_frame_mut::<StackFrameStringLength>();
    *frame.result_ptr = result;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringNext<'a> {
    common: StackFrameCommon,
    string: StringObject,
    index: Option<&'a mut isize>,
    rune: Option<&'a mut i32>,
    found: &'a mut bool,
    count: &'a mut usize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_next(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameStringNext>();

    let s = frame.string.to_str().unwrap();
    let index = *frame.count;
    if let Some(c) = s.chars().nth(index) {
        if let Some(p) = frame.index.as_mut() {
            **p = index as isize;
        }
        if let Some(p) = frame.rune.as_mut() {
            **p = c as i32;
        }
        *frame.found = true;
        *frame.count = index + 1;
    } else {
        *frame.found = false;
    }

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameStringSubstr<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    base: StringObject,
    low: isize,
    high: isize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_substr(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringSubstr>();

    let low = {
        let low = frame.low;
        if low < 0 {
            assert_eq!(low, -1);
            0
        } else {
            low as usize
        }
    };

    let high = {
        let high = frame.high;
        if high < 0 {
            assert_eq!(high, -1);
            frame.base.len_in_bytes()
        } else {
            high as usize
        }
    };

    assert!(low <= high);
    let len = high - low;

    let mut builder = ctx
        .global_context()
        .process(|mut global_context| StringObject::builder(len, global_context.allocator()));

    builder.append_bytes(&frame.base.as_bytes()[low..high]);

    let frame = ctx.stack_frame_mut::<StackFrameStringSubstr>();
    *frame.result_ptr = builder.build();

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
    fn test_gox5_string_length() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameStringLength>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let s = builder.build();

        let frame = ctx.stack_frame_mut::<StackFrameStringLength>();
        frame.string = s;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_length(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 5);
    }

    #[test]
    fn test_gox5_string_length_empty() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameStringLength>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let mut allocator = MockObjectAllocator::new();
        let builder = StringObject::builder(0, &mut allocator);
        let s = builder.build();

        let frame = ctx.stack_frame_mut::<StackFrameStringLength>();
        frame.string = s;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_length(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 0);
    }

    #[test]
    fn test_gox5_string_new_from_rune() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameStringNewFromRune>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRune>();
        frame.rune = 'A' as usize;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_new_from_rune(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert_eq!(res.as_bytes(), b"A");
    }

    #[test]
    fn test_gox5_string_new_from_rune_multibyte() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameStringNewFromRune>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameStringNewFromRune>();
        frame.rune = '日' as usize;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_new_from_rune(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert_eq!(res.as_bytes(), "日".as_bytes());
    }

    #[test]
    fn test_gox5_string_substr() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameStringSubstr>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let base = builder.build();

        let frame = ctx.stack_frame_mut::<StackFrameStringSubstr>();
        frame.base = base;
        frame.low = 1;
        frame.high = 4;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_substr(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert_eq!(res.as_bytes(), b"ell");
    }

    #[test]
    fn test_gox5_string_substr_full() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameStringSubstr>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let base = builder.build();

        let frame = ctx.stack_frame_mut::<StackFrameStringSubstr>();
        frame.base = base;
        frame.low = -1;
        frame.high = -1;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_substr(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert_eq!(res.as_bytes(), b"hello");
    }

    #[test]
    fn test_gox5_string_append() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameStringAppend>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let mut allocator = MockObjectAllocator::new();
        let mut builder1 = StringObject::builder(5, &mut allocator);
        builder1.append_bytes(b"hello");
        let lhs = builder1.build();
        let mut builder2 = StringObject::builder(5, &mut allocator);
        builder2.append_bytes(b"world");
        let rhs = builder2.build();

        let frame = ctx.stack_frame_mut::<StackFrameStringAppend>();
        frame.lhs = lhs;
        frame.rhs = rhs;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_append(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert_eq!(res.as_bytes(), b"helloworld");
    }
}
