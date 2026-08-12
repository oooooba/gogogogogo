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

// Tables from Go's unicode/utf8 decodeRuneInStringSlow.
const UTF8_FIRST: [u8; 256] = [
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
    0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1,
    0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1,
    0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1,
    0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1,
    0xF1, 0xF1, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
    0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
    0x13, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x23, 0x03, 0x03,
    0x34, 0x04, 0x04, 0x04, 0x44, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1,
];
const UTF8_ACCEPT_RANGES: [(u8, u8); 5] = [
    (0x80, 0xBF),
    (0xA0, 0xBF),
    (0x80, 0x9F),
    (0x90, 0xBF),
    (0x80, 0x8F),
];
const UTF8_LOCB: u8 = 0x80;
const UTF8_HICB: u8 = 0xBF;

fn decode_rune_next(bytes: &[u8]) -> (u32, usize) {
    let n = bytes.len();
    if n < 1 {
        return (0xFFFD, 0);
    }
    let s0 = bytes[0];
    let x = UTF8_FIRST[s0 as usize];
    if x >= 0xF0 {
        if x == 0xF0 {
            return (s0 as u32, 1);
        }
        return (0xFFFD, 1);
    }
    let sz = (x & 7) as usize;
    let (lo, hi) = UTF8_ACCEPT_RANGES[(x >> 4) as usize];
    if n < sz {
        return (0xFFFD, 1);
    }
    let s1 = bytes[1];
    if s1 < lo || hi < s1 {
        return (0xFFFD, 1);
    }
    if sz <= 2 {
        return (((s0 as u32 & 0x1F) << 6) | (s1 as u32 & 0x3F), 2);
    }
    let s2 = bytes[2];
    if !(UTF8_LOCB..=UTF8_HICB).contains(&s2) {
        return (0xFFFD, 1);
    }
    if sz <= 3 {
        return (
            ((s0 as u32 & 0x0F) << 12) | ((s1 as u32 & 0x3F) << 6) | (s2 as u32 & 0x3F),
            3,
        );
    }
    let s3 = bytes[3];
    if !(UTF8_LOCB..=UTF8_HICB).contains(&s3) {
        return (0xFFFD, 1);
    }
    (
        ((s0 as u32 & 0x07) << 18)
            | ((s1 as u32 & 0x3F) << 12)
            | ((s2 as u32 & 0x3F) << 6)
            | (s3 as u32 & 0x3F),
        4,
    )
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_next(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameStringNext>();

    let bytes = frame.string.as_bytes();
    let index = *frame.count;
    if index < bytes.len() {
        let (rune, size) = decode_rune_next(&bytes[index..]);
        if let Some(p) = frame.index.as_mut() {
            **p = index as isize;
        }
        if let Some(p) = frame.rune.as_mut() {
            **p = rune as i32;
        }
        *frame.found = true;
        *frame.count = index + size;
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

#[repr(C)]
struct StackFrameStringSearchByte<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    string: StringObject,
    byte: u8,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_search_byte(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringSearchByte>();
    let result = frame
        .string
        .as_bytes()
        .iter()
        .position(|&b| b == frame.byte)
        .map_or(-1, |i| isize::try_from(i).unwrap());

    let frame = ctx.stack_frame_mut::<StackFrameStringSearchByte>();
    *frame.result_ptr = result;

    ctx.pop_frame()
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
struct StackFrameStringSearchString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    lhs: StringObject,
    rhs: StringObject,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_string_search_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameStringSearchString>();
    let result = index_of(frame.lhs.as_bytes(), frame.rhs.as_bytes());

    let frame = ctx.stack_frame_mut::<StackFrameStringSearchString>();
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

    fn make_string(ctx: &mut LightWeightThreadContext, bytes: &[u8]) -> StringObject {
        let mut builder = ctx.global_context().process(|mut global_context| {
            StringObject::builder(bytes.len(), global_context.allocator())
        });
        builder.append_bytes(bytes);
        builder.build()
    }

    struct NextOut {
        index: isize,
        rune: i32,
        found: bool,
        count: usize,
    }

    fn step_next(
        ctx: &mut LightWeightThreadContext,
        string_obj: StringObject,
        count_in: usize,
        use_index: bool,
        use_rune: bool,
    ) -> NextOut {
        let mut index_slot: isize = -1;
        let mut rune_slot: i32 = -1;
        let mut found_slot: bool = false;
        let mut count_slot: usize = count_in;

        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameStringNext>());
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());

        {
            let frame = ctx.stack_frame_mut::<StackFrameStringNext>();
            frame.string = string_obj;
            frame.index = if use_index {
                Some(&mut index_slot)
            } else {
                None
            };
            frame.rune = if use_rune { Some(&mut rune_slot) } else { None };
            frame.found = &mut found_slot;
            frame.count = &mut count_slot;
        }

        let result = gox5_string_next(ctx);
        assert_eq!(result, FunctionObject::new_null());

        NextOut {
            index: index_slot,
            rune: rune_slot,
            found: found_slot,
            count: count_slot,
        }
    }

    fn iterate(ctx: &mut LightWeightThreadContext, bytes: &[u8]) -> Vec<(isize, i32, bool)> {
        let s = make_string(ctx, bytes);
        let mut steps = Vec::new();
        let mut count = 0;
        loop {
            let out = step_next(ctx, s.clone(), count, true, true);
            count = out.count;
            steps.push((out.index, out.rune, out.found));
            if !out.found {
                break;
            }
        }
        steps
    }

    #[test]
    fn test_gox5_string_next_ascii() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"ABC");
        assert_eq!(
            steps,
            vec![
                (0, 'A' as i32, true),
                (1, 'B' as i32, true),
                (2, 'C' as i32, true),
                (-1, -1, false)
            ]
        );
    }

    #[test]
    fn test_gox5_string_next_multibyte() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, "A€z".as_bytes());
        assert_eq!(
            steps,
            vec![
                (0, 'A' as i32, true),
                (1, 0x20AC, true),
                (4, 'z' as i32, true),
                (-1, -1, false)
            ]
        );
    }

    #[test]
    fn test_gox5_string_next_japanese() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, "日本語".as_bytes());
        assert_eq!(
            steps,
            vec![
                (0, 0x65E5, true),
                (3, 0x672C, true),
                (6, 0x8A9E, true),
                (-1, -1, false)
            ]
        );
    }

    #[test]
    fn test_gox5_string_next_four_byte() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"\xF0\x90\x80\x80z");
        assert_eq!(
            steps,
            vec![(0, 0x10000, true), (4, 'z' as i32, true), (-1, -1, false)]
        );
    }

    #[test]
    fn test_gox5_string_next_invalid_byte() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"A\xFFz");
        assert_eq!(
            steps,
            vec![
                (0, 'A' as i32, true),
                (1, 0xFFFD, true),
                (2, 'z' as i32, true),
                (-1, -1, false)
            ]
        );
    }

    #[test]
    fn test_gox5_string_next_truncated_sequence() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"\xE2\x82");
        assert_eq!(
            steps,
            vec![(0, 0xFFFD, true), (1, 0xFFFD, true), (-1, -1, false)]
        );
    }

    #[test]
    fn test_gox5_string_next_surrogate() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"\xED\xA0\x80");
        assert_eq!(
            steps,
            vec![
                (0, 0xFFFD, true),
                (1, 0xFFFD, true),
                (2, 0xFFFD, true),
                (-1, -1, false)
            ]
        );
    }

    #[test]
    fn test_gox5_string_next_empty() {
        let (mut ctx, _gc) = create_ctx();
        let steps = iterate(&mut ctx, b"");
        assert_eq!(steps, vec![(-1, -1, false)]);
    }

    #[test]
    fn test_gox5_string_next_no_index_no_rune() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, "A€z".as_bytes());

        let mut count = 0;
        let mut found_sequence = Vec::new();
        loop {
            let out = step_next(&mut ctx, s.clone(), count, false, false);
            count = out.count;
            found_sequence.push(out.found);
            if !out.found {
                break;
            }
        }
        assert_eq!(found_sequence, vec![true, true, true, false]);
        assert_eq!(count, 5);
    }

    fn search_byte(ctx: &mut LightWeightThreadContext, s: &StringObject, byte: u8) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameStringSearchByte>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameStringSearchByte>();
        frame.string = s.clone();
        frame.byte = byte;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_search_byte(ctx);
        assert_eq!(result, FunctionObject::new_null());
        unsafe { *result_raw }
    }

    #[test]
    fn test_gox5_string_search_byte_found() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello");
        assert_eq!(search_byte(&mut ctx, &s, b'l'), 2);
    }

    #[test]
    fn test_gox5_string_search_byte_first() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello");
        assert_eq!(search_byte(&mut ctx, &s, b'h'), 0);
    }

    #[test]
    fn test_gox5_string_search_byte_not_found() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello");
        assert_eq!(search_byte(&mut ctx, &s, b'x'), -1);
    }

    #[test]
    fn test_gox5_string_search_byte_empty() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"");
        assert_eq!(search_byte(&mut ctx, &s, b'a'), -1);
    }

    fn search_string(
        ctx: &mut LightWeightThreadContext,
        s: &StringObject,
        sep: &StringObject,
    ) -> isize {
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<isize>());
        let result_raw = ctx.stack_pointer() as *mut isize;
        ctx.grow_stack(mem::size_of::<StackFrameStringSearchString>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameStringSearchString>();
        frame.lhs = s.clone();
        frame.rhs = sep.clone();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_string_search_string(ctx);
        assert_eq!(result, FunctionObject::new_null());
        unsafe { *result_raw }
    }

    #[test]
    fn test_gox5_string_search_string() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello world");
        let needle = make_string(&mut ctx, b"world");
        assert_eq!(search_string(&mut ctx, &s, &needle), 6);
    }

    #[test]
    fn test_gox5_string_search_string_not_found() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello");
        let needle = make_string(&mut ctx, b"xyz");
        assert_eq!(search_string(&mut ctx, &s, &needle), -1);
    }

    #[test]
    fn test_gox5_string_search_string_empty() {
        let (mut ctx, _gc) = create_ctx();
        let s = make_string(&mut ctx, b"hello");
        let empty = make_string(&mut ctx, b"");
        assert_eq!(search_string(&mut ctx, &s, &empty), 0);
    }
}
