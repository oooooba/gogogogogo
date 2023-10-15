use std::ffi;
use std::mem;
use std::mem::ManuallyDrop;
use std::ptr;
use std::slice;

use super::channel::Channel;
use super::create_light_weight_thread_context;
use super::interface::Interface;
use super::map::Map;
use super::type_id::TypeId;
use super::LightWeightThreadContext;
use super::ObjectPtr;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct FunctionObject(*const ());

impl FunctionObject {
    pub fn from_user_function(user_function: UserFunction) -> Self {
        FunctionObject(user_function.0 as *const ())
    }

    pub fn from_closure_layout_ptr(closure_layout_ptr: *const ()) -> Self {
        let addr = closure_layout_ptr as usize;
        let flag = 1 << 63;
        assert_eq!(addr & flag, 0);
        FunctionObject((addr | flag) as *const ())
    }

    pub fn new_null() -> Self {
        FunctionObject(ptr::null_mut())
    }

    pub fn extract_user_function(&self) -> (UserFunction, Option<*mut ()>) {
        let addr = self.0 as usize;
        let flag = 1 << 63;
        if (addr & flag) == 0 {
            let func = unsafe { mem::transmute::<*const (), UserFunction>(self.0) };
            return (func, None);
        }
        let ptr = (addr & !flag) as *mut () as *mut ClosureLayout;
        unsafe {
            let closure_layout = &mut *ptr;
            let func = closure_layout.func.clone();
            let object_ptrs = closure_layout.object_ptrs.as_mut_ptr() as *mut ();
            (func, Some(object_ptrs))
        }
    }
}

type UserFunctionInner = unsafe extern "C" fn(&mut LightWeightThreadContext) -> FunctionObject;

#[derive(Clone)]
#[repr(C)]
pub struct UserFunction(UserFunctionInner);

impl UserFunction {
    pub fn new(user_function: UserFunctionInner) -> Self {
        UserFunction(user_function)
    }

    pub fn invoke(&self, ctx: &mut LightWeightThreadContext) -> FunctionObject {
        unsafe { self.0(ctx) }
    }
}

impl PartialEq<UserFunctionInner> for UserFunction {
    fn eq(&self, other: &UserFunctionInner) -> bool {
        let lhs = self.0 as *const ();
        let rhs = *other as *const ();
        lhs == rhs
    }
}

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct StringObject(*const libc::c_char);

#[repr(C)]
struct Value {
    object: *mut ObjectPtr,
}

impl Value {
    fn new(object: *mut ObjectPtr) -> Self {
        Value { object }
    }
}

#[repr(C)]
struct Func {
    name: StringObject,
    function: UserFunction,
}

#[repr(C)]
struct StackFrameCommon {
    resume_func: FunctionObject,
    prev_stack_pointer: *mut StackFrame,
    free_vars: *mut (),
}

#[repr(C)]
struct Slice {
    addr: *mut (),
    size: usize,
    capacity: usize,
}

impl Slice {
    fn new(addr: *mut (), size: usize, capacity: usize) -> Self {
        Slice {
            addr,
            size,
            capacity,
        }
    }

    fn as_raw_slice<T>(&self) -> Option<&mut [T]> {
        if self.capacity == 0 {
            return None;
        }
        assert_ne!(self.addr, ptr::null_mut());
        unsafe {
            Some(slice::from_raw_parts_mut(
                self.addr as *mut T,
                self.capacity,
            ))
        }
    }
}

#[repr(C)]
struct StackFrameAppend {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    base: Slice,
    elements: Slice,
}

pub fn append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (base, elements, result) = unsafe {
        let (base_ptr, elements_ptr, result_ptr) = {
            let stack_frame = &mut ctx.stack_frame_mut().append;
            (
                &mut stack_frame.base as *mut Slice,
                &mut stack_frame.elements as *mut Slice,
                stack_frame.result_ptr as *mut Slice,
            )
        };
        (&mut *base_ptr, &mut *elements_ptr, &mut *result_ptr)
    };

    let new_size = base.size + elements.size;
    let has_space = new_size < base.capacity;
    if has_space {
        unsafe {
            ptr::copy_nonoverlapping(base, result, 1);
        }
    } else {
        let new_capacity = new_size * 2;
        let buffer_size = new_capacity * mem::size_of::<isize>();
        let ptr = ctx.global_context().process(|mut global_context| {
            global_context
                .allocator()
                .allocate(buffer_size * mem::size_of::<isize>(), |_ptr| {})
        });
        unsafe {
            ptr::write_bytes(ptr as *mut u8, 0, buffer_size);
        }
        result.addr = ptr;
        result.size = new_size;
        result.capacity = new_capacity;
    }

    if let Some(base_raw_slice) = base.as_raw_slice::<isize>() {
        if !has_space {
            let result_raw_slice = result.as_raw_slice().unwrap();
            result_raw_slice[..base_raw_slice.len()]
                .clone_from_slice(&base_raw_slice[..base_raw_slice.len()]);
        }
    }

    if let Some(elements_raw_slice) = elements.as_raw_slice::<isize>() {
        let result_raw_slice = result.as_raw_slice().unwrap();
        for i in 0..elements_raw_slice.len() {
            result_raw_slice[base.size + i] = elements_raw_slice[i];
        }
    }

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameConcat {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    lhs: StringObject,
    rhs: StringObject,
}

pub fn concat(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (lhs, rhs, result) = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().concat;
        let lhs = stack_frame.lhs.clone();
        let rhs = stack_frame.rhs.clone();
        (lhs, rhs, &mut (*stack_frame.result_ptr))
    };

    let concat_string = unsafe {
        let lhs = ffi::CStr::from_ptr(lhs.0).to_str().unwrap();
        let rhs = ffi::CStr::from_ptr(rhs.0).to_str().unwrap();
        format!("{lhs}{rhs}")
    };

    let len = concat_string.len();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
    });
    let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };
    let src_bytes = concat_string.as_bytes();

    for i in 0..len {
        dst_bytes[i] = src_bytes[i] as libc::c_char;
    }
    dst_bytes[len] = 0;

    let new_string_object = StringObject(ptr);
    *result = new_string_object;

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameFuncForPc {
    common: StackFrameCommon,
    result_ptr: *mut *const Func,
    param0: usize,
}

pub fn func_for_pc(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (param0, result) = unsafe {
        let (param0_ptr, result_ptr) = {
            let stack_frame = &mut ctx.stack_frame_mut().func_for_pc;
            (
                &stack_frame.param0 as *const usize,
                stack_frame.result_ptr as *mut *const Func,
            )
        };
        (*param0_ptr, &mut *result_ptr)
    };

    extern "C" {
        fn runtime_info_get_funcs_count() -> libc::size_t;
        fn runtime_info_refer_func(i: libc::size_t) -> *const Func;
    }

    *result = ptr::null();
    unsafe {
        for i in 0..runtime_info_get_funcs_count() {
            let func = runtime_info_refer_func(i);
            if (*func).function.0 as usize == param0 {
                *result = func;
                break;
            }
        }
    }

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameFuncName {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    param0: *const Func,
}

pub fn func_name(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (param0, result) = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().func_name;
        (&*stack_frame.param0, &mut *stack_frame.result_ptr)
    };

    let s = param0.name.clone();

    unsafe {
        ptr::copy_nonoverlapping(&s, result, 1);
    }

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeChan {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

/// temporarily, exported for unit test
pub fn allocate_channel(ctx: &mut LightWeightThreadContext, capacity: usize) -> *mut Channel {
    let object_size = mem::size_of::<Channel>();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(object_size, |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut Channel)
            }) as *mut Channel
    });

    let channel = Channel::new(capacity);
    unsafe {
        ptr::copy_nonoverlapping(&channel, ptr, 1);
    }
    mem::forget(channel);

    ptr
}

pub fn make_chan(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let size = unsafe {
        let stack_frame = &ctx.stack_frame().make_chan;
        stack_frame.size
    };
    let ptr = allocate_channel(ctx, size);
    let ptr = ObjectPtr(ptr as *mut ());
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_chan;
        *stack_frame.result_ptr = ptr;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeClosure {
    common: StackFrameCommon,
    result_ptr: *mut FunctionObject,
    user_function: UserFunction,
    num_object_ptrs: usize,
    object_ptrs: [ObjectPtr; 0],
}

#[repr(C)]
struct ClosureLayout {
    func: UserFunction,
    object_ptrs: Vec<ObjectPtr>,
}

pub fn make_closure(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>(), |_ptr| {})
    });
    unsafe {
        let ptr = ptr as *mut ClosureLayout;
        let closure_layout = &mut *ptr;

        let stack_frame = &mut ctx.stack_frame_mut().make_closure;

        closure_layout.func = stack_frame.user_function.clone();

        let num_object_ptrs = stack_frame.num_object_ptrs;
        let object_ptrs = stack_frame.object_ptrs.as_mut_ptr();
        let object_ptrs = slice::from_raw_parts(object_ptrs, num_object_ptrs).to_vec();

        let prev_object_ptrs = mem::replace(&mut closure_layout.object_ptrs, object_ptrs);
        mem::forget(prev_object_ptrs);
    };
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_closure;
        *stack_frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeInterface {
    common: StackFrameCommon,
    result_ptr: *mut Interface,
    receiver: ObjectPtr,
    type_id: TypeId,
}

pub fn make_interface(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = unsafe { &ctx.stack_frame().make_interface };

    let interface = Interface::new(frame.receiver.clone(), frame.type_id);
    unsafe {
        ptr::copy_nonoverlapping(&interface, frame.result_ptr, 1);
    }
    mem::forget(interface);
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeMap {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    key_type_size: usize,
    value_type_size: usize,
}

pub fn allocate_map(
    ctx: &mut LightWeightThreadContext,
    key_type_size: usize,
    value_type_size: usize,
) -> *mut Map {
    let object_size = mem::size_of::<Map>();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(object_size, |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut Map)
            }) as *mut Map
    });

    let map = Map::new(key_type_size, value_type_size);
    unsafe {
        ptr::copy_nonoverlapping(&map, ptr, 1);
    }
    mem::forget(map);
    ptr
}

pub fn make_map(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (key_type_size, value_type_size) = unsafe {
        let stack_frame = &ctx.stack_frame().make_map;
        (stack_frame.key_type_size, stack_frame.value_type_size)
    };
    let ptr = allocate_map(ctx, key_type_size, value_type_size);
    let ptr = ObjectPtr(ptr as *mut ());
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_map;
        *stack_frame.result_ptr = ptr;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeStringFromByteSlice {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    byte_slice: Slice,
}

pub fn make_string_from_byte_slice(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let len = unsafe {
        let stack_frame = &ctx.stack_frame().make_string_from_byte_slice;
        stack_frame.byte_slice.size
    };

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
    });

    let byte_slice = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_string_from_byte_slice;
        &stack_frame.byte_slice
    };

    let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };
    if let Some(src_bytes) = byte_slice.as_raw_slice::<libc::c_char>() {
        dst_bytes[..len].clone_from_slice(&src_bytes[..len]);
    }
    dst_bytes[len] = 0;

    let result = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_string_from_byte_slice;
        &mut (*stack_frame.result_ptr)
    };

    let new_string_object = StringObject(ptr);
    *result = new_string_object;

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeStringFromRune {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    rune: usize,
}

fn make_string(ctx: &mut LightWeightThreadContext, chars: &[char]) -> StringObject {
    let len = chars.iter().fold(0, |acc, c| acc + c.len_utf8());
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
    });
    let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };

    chars.iter().fold(0, |acc, c| {
        let c_len = c.len_utf8();
        let mut src_bytes = vec![0; c_len];
        let _ = c.encode_utf8(&mut src_bytes);
        for i in 0..c_len {
            dst_bytes[acc + i] = src_bytes[i] as libc::c_char;
        }
        acc + c_len
    });
    dst_bytes[len] = 0;

    StringObject(ptr)
}

pub fn make_string_from_rune(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (rune, result) = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_string_from_rune;
        (stack_frame.rune, &mut (*stack_frame.result_ptr))
    };

    assert!(rune <= std::u32::MAX as usize);
    let chars = vec![char::from_u32(rune as u32).unwrap()];

    let new_string_object = make_string(ctx, &chars);
    *result = new_string_object;

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeStringFromRuneSlice {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    rune_slice: Slice,
}

pub fn make_string_from_rune_slice(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let rune_slice = unsafe {
        let stack_frame = &ctx.stack_frame().make_string_from_rune_slice;
        &stack_frame.rune_slice
    };

    let chars = if let Some(src_runes) = rune_slice.as_raw_slice::<u32>() {
        src_runes[..rune_slice.size]
            .iter()
            .map(|rune| char::from_u32(*rune).unwrap())
            .collect()
    } else {
        Vec::new()
    };

    let new_string_object = make_string(ctx, &chars);
    let result = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().make_string_from_byte_slice;
        &mut (*stack_frame.result_ptr)
    };
    *result = new_string_object;

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMapGet {
    common: StackFrameCommon,
    result_ptr: *mut usize,
    map: ObjectPtr,
    key: usize,
}

pub fn map_get(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (map_ptr, key) = unsafe {
        let stack_frame = &ctx.stack_frame().map_get;
        (&stack_frame.map, &stack_frame.key)
    };
    let value = if map_ptr.is_null() {
        unimplemented!()
    } else {
        let map = map_ptr.as_ref::<Map>();
        map.get(key).unwrap()
    };
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().map_get;
        *stack_frame.result_ptr = value;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct MapGetCheckedResult {
    value: usize,
    found: bool,
}

#[repr(C)]
struct StackFrameMapGetChecked {
    common: StackFrameCommon,
    result_ptr: *mut MapGetCheckedResult,
    map: ObjectPtr,
    key: usize,
}

pub fn map_get_checked(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (map_ptr, key) = unsafe {
        let stack_frame = &ctx.stack_frame().map_get_checked;
        (&stack_frame.map, &stack_frame.key)
    };
    let (value, found) = if map_ptr.is_null() {
        unimplemented!()
    } else {
        let map = map_ptr.as_ref::<Map>();
        map.get(key).map_or((0, false), |v| (v, true))
    };
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().map_get_checked;
        *stack_frame.result_ptr = MapGetCheckedResult { value, found };
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMapLen {
    common: StackFrameCommon,
    result_ptr: *mut usize,
    map: ObjectPtr,
}

pub fn map_len(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let map_ptr = unsafe {
        let stack_frame = &ctx.stack_frame().map_len;
        &stack_frame.map
    };
    let len = if map_ptr.is_null() {
        0
    } else {
        let map = map_ptr.as_ref::<Map>();
        map.len()
    };
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().map_len;
        *stack_frame.result_ptr = len;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMapSet {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: usize,
    value: usize,
}

pub fn map_set(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (mut map_ptr, key, value) = unsafe {
        let stack_frame = &ctx.stack_frame().map_set;
        (
            stack_frame.map.clone(),
            &stack_frame.key,
            &stack_frame.value,
        )
    };
    if map_ptr.is_null() {
        unimplemented!()
    } else {
        let map = map_ptr.as_mut::<Map>();
        map.set(key, *value)
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameNew {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

pub fn new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let size = unsafe {
        let stack_frame = &ctx.stack_frame().new;
        stack_frame.size
    };
    let ptr = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
    unsafe {
        ptr::write_bytes(ptr as *mut u8, 0, size);
    }
    let ptr = ObjectPtr(ptr);
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().new;
        *stack_frame.result_ptr = ptr;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameRecv {
    common: StackFrameCommon,
    result_ptr: *mut isize,
    channel: ObjectPtr,
}

pub fn recv(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let mut channel = unsafe {
        let stack_frame = &ctx.stack_frame().recv;
        stack_frame.channel.clone()
    };
    let channel = channel.as_mut::<Channel>();
    let data = channel.recv()?;
    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().recv;
        *stack_frame.result_ptr = data;
    }
    Some(leave_runtime_api(ctx))
}

pub fn schedule(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameSend {
    common: StackFrameCommon,
    channel: ObjectPtr,
    data: isize,
}

pub fn send(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let (mut channel, data) = unsafe {
        let stack_frame = &ctx.stack_frame().send;
        (stack_frame.channel.clone(), stack_frame.data)
    };
    let channel = channel.as_mut::<Channel>();
    channel.send(&data)?;
    Some(leave_runtime_api(ctx))
}

#[repr(C)]
struct StackFrameSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [(); 0],
}

pub fn spawn(ctx: &mut LightWeightThreadContext) -> (FunctionObject, LightWeightThreadContext) {
    let new_ctx = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().spawn;

        let entry_func = stack_frame.func.clone();
        let result_size = stack_frame.result_size;
        let arg_buffer_ptr = ObjectPtr(stack_frame.arg_buffer.as_mut_ptr());
        let num_arg_buffer_words = stack_frame.num_arg_buffer_words;
        let global_context = ctx.global_context().dupulicate();

        let mut new_ctx = create_light_weight_thread_context(global_context);
        new_ctx.set_up(
            entry_func,
            result_size,
            arg_buffer_ptr,
            num_arg_buffer_words,
        );
        new_ctx
    };
    (leave_runtime_api(ctx), new_ctx)
}

#[repr(C)]
struct StackFrameSplit {
    common: StackFrameCommon,
    result_ptr: *mut Slice,
    param0: StringObject,
    param1: StringObject,
}

pub fn split(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (param0, param1) = unsafe {
        let (param0_ptr, param1_ptr) = {
            let stack_frame = &ctx.stack_frame().split;
            (
                &stack_frame.param0 as *const StringObject,
                &stack_frame.param1 as *const StringObject,
            )
        };
        (&*param0_ptr, &*param1_ptr)
    };

    let (target, sep) = unsafe {
        let target = ffi::CStr::from_ptr(param0.0).to_str().unwrap();
        let sep = ffi::CStr::from_ptr(param1.0).to_str().unwrap();
        (target, sep)
    };
    let words: Vec<&str> = target.split(sep).collect();

    let slice = {
        let size = words.len();
        let capacity = size;
        let buffer_size = capacity * mem::size_of::<StringObject>();
        let addr = ctx.global_context().process(|mut global_context| {
            global_context.allocator().allocate(buffer_size, |_ptr| {})
        });
        Slice::new(addr, size, capacity)
    };

    if let Some(raw_slice) = slice.as_raw_slice::<StringObject>() {
        for i in 0..raw_slice.len() {
            let src_bytes = words[i].as_bytes();
            let len = src_bytes.len();

            let ptr = ctx.global_context().process(|mut global_context| {
                global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
            });
            let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };

            for j in 0..len {
                dst_bytes[j] = src_bytes[j] as libc::c_char;
            }
            dst_bytes[len] = 0;

            raw_slice[i] = StringObject(ptr)
        }
    }

    unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().split;
        ptr::copy_nonoverlapping(&slice, stack_frame.result_ptr, 1);
    }
    mem::forget(slice);

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameStrview {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    base: StringObject,
    low: isize,
    high: isize,
}

pub fn strview(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (base, low, high, result) = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().strview;
        let base = stack_frame.base.clone();
        let low = stack_frame.low;
        let high = stack_frame.high;
        (base, low, high, &mut (*stack_frame.result_ptr))
    };

    let base_string = unsafe { ffi::CStr::from_ptr(base.0).to_str().unwrap() };

    let low = if low < 0 {
        assert_eq!(low, -1);
        0
    } else {
        low as usize
    };

    let high = if high < 0 {
        assert_eq!(high, -1);
        base_string.len()
    } else {
        high as usize
    };

    assert!(low <= high);
    let len = high - low;
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
    });
    let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };
    let src_bytes = base_string.as_bytes();

    for i in 0..len {
        dst_bytes[i] = src_bytes[low + i] as libc::c_char;
    }
    dst_bytes[len] = 0;

    let new_string_object = StringObject(ptr);
    *result = new_string_object;

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameValueOf {
    common: StackFrameCommon,
    result_ptr: *mut Value,
    param0: Interface,
}

pub fn value_of(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (param0, result) = unsafe {
        let (param0_ptr, result_ptr) = {
            let stack_frame = &mut ctx.stack_frame_mut().value_of;
            (
                &mut stack_frame.param0 as *mut Interface,
                stack_frame.result_ptr as *mut Value,
            )
        };
        (&mut *param0_ptr, &mut *result_ptr)
    };

    let object = unsafe {
        let function_object_ptr = param0.receiver().0 as *const FunctionObject;
        (*function_object_ptr).0 as *mut ObjectPtr
    };

    let value = Value::new(object);
    unsafe {
        ptr::copy_nonoverlapping(&value, result, 1);
    }
    mem::forget(value);

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameValuePointer {
    common: StackFrameCommon,
    result_ptr: *mut isize,
    param0: Value,
}

pub fn value_pointer(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (param0, result) = unsafe {
        let (param0_ptr, result_ptr) = {
            let stack_frame = &mut ctx.stack_frame_mut().value_pointer;
            (
                &mut stack_frame.param0 as *const Value,
                stack_frame.result_ptr as *mut isize,
            )
        };
        (&*param0_ptr, &mut *result_ptr)
    };

    *result = param0.object as isize;

    leave_runtime_api(ctx)
}

fn leave_runtime_api(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (prev_stack_pointer, resumt_func) = unsafe {
        let stack_frame = &mut ctx.stack_frame_mut().common;
        (
            stack_frame.prev_stack_pointer,
            stack_frame.resume_func.clone(),
        )
    };
    ctx.update_stack_pointer(prev_stack_pointer);
    resumt_func
}

#[repr(C)]
pub union StackFrame {
    pub words: [*mut (); 0],
    common: ManuallyDrop<StackFrameCommon>,
    append: ManuallyDrop<StackFrameAppend>,
    concat: ManuallyDrop<StackFrameConcat>,
    func_for_pc: ManuallyDrop<StackFrameFuncForPc>,
    func_name: ManuallyDrop<StackFrameFuncName>,
    make_chan: ManuallyDrop<StackFrameMakeChan>,
    make_closure: ManuallyDrop<StackFrameMakeClosure>,
    make_interface: ManuallyDrop<StackFrameMakeInterface>,
    make_map: ManuallyDrop<StackFrameMakeMap>,
    make_string_from_byte_slice: ManuallyDrop<StackFrameMakeStringFromByteSlice>,
    make_string_from_rune: ManuallyDrop<StackFrameMakeStringFromRune>,
    make_string_from_rune_slice: ManuallyDrop<StackFrameMakeStringFromRuneSlice>,
    map_get: ManuallyDrop<StackFrameMapGet>,
    map_get_checked: ManuallyDrop<StackFrameMapGetChecked>,
    map_len: ManuallyDrop<StackFrameMapLen>,
    map_set: ManuallyDrop<StackFrameMapSet>,
    new: ManuallyDrop<StackFrameNew>,
    recv: ManuallyDrop<StackFrameRecv>,
    send: ManuallyDrop<StackFrameSend>,
    split: ManuallyDrop<StackFrameSplit>,
    spawn: ManuallyDrop<StackFrameSpawn>,
    strview: ManuallyDrop<StackFrameStrview>,
    value_of: ManuallyDrop<StackFrameValueOf>,
    value_pointer: ManuallyDrop<StackFrameValuePointer>,
}
