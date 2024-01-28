pub(crate) mod map;

use std::ffi;
use std::mem;
use std::ptr;
use std::slice;

use crate::gox5_run_defers;

use super::channel::Channel;
use super::create_light_weight_thread_context;
use super::interface::Interface;
use super::type_id::TypeId;
use super::ClosureLayout;
use super::FunctionObject;
use super::LightWeightThreadContext;
use super::ObjectPtr;
use super::StackFrame;
use super::StackFrameCommon;
use super::UserFunction;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct StringObject(*const libc::c_char);

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
    let (base, elements) = {
        let stack_frame = ctx.stack_frame::<StackFrameAppend>();
        (&stack_frame.base, &stack_frame.elements)
    };

    let mut result = Slice::new(ptr::null_mut(), 0, 0);

    let new_size = base.size + elements.size;
    let has_space = new_size < base.capacity;
    if has_space {
        unsafe {
            ptr::copy_nonoverlapping(base, &mut result, 1);
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

    let stack_frame = ctx.stack_frame_mut::<StackFrameAppend>();
    unsafe {
        let p = stack_frame.result_ptr as *mut Slice;
        ptr::copy_nonoverlapping(&result, &mut *p, 1);
    }

    ctx.leave()
}

#[repr(C)]
struct StackFrameConcat {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    lhs: StringObject,
    rhs: StringObject,
}

pub fn concat(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (lhs, rhs, result) = {
        let stack_frame = &mut ctx.stack_frame::<StackFrameConcat>();
        let lhs = stack_frame.lhs.clone();
        let rhs = stack_frame.rhs.clone();
        (lhs, rhs, unsafe { &mut (*stack_frame.result_ptr) })
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

    ctx.leave()
}

struct Deferred {
    prev_deferred: *const (),
    target_stack_pointer: *const (),
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: *const *const (),
}

#[repr(C)]
struct StackFrameDefer {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [(); 0],
}

pub fn defer(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let (func, result_size, num_arg_buffer_words) = {
        let stack_frame = ctx.stack_frame::<StackFrameDefer>();
        (
            stack_frame.func.clone(),
            stack_frame.result_size,
            stack_frame.num_arg_buffer_words,
        )
    };

    let dst_arg_buffer = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<*const ()>() * num_arg_buffer_words, |_| {})
            as *mut *const ()
    });

    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameDefer>();
        let src = stack_frame.arg_buffer.as_ptr() as *const *const ();
        ptr::copy_nonoverlapping(src, dst_arg_buffer, num_arg_buffer_words);
    }

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<Deferred>(), |_| {}) as *mut Deferred
    });
    let prev_deferred = ctx.deferred_list();
    let deferred = Deferred {
        prev_deferred,
        target_stack_pointer: ctx.stack_pointer(),
        func,
        result_size,
        num_arg_buffer_words,
        arg_buffer: dst_arg_buffer,
    };
    unsafe {
        ptr::copy_nonoverlapping(&deferred, ptr, 1);
    }
    mem::forget(deferred);

    ctx.update_deferred_list(ptr as *const ());

    ctx.leave()
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
    let size = {
        let stack_frame = ctx.stack_frame::<StackFrameMakeChan>();
        stack_frame.size
    };
    let ptr = allocate_channel(ctx, size);
    let ptr = ObjectPtr(ptr as *mut ());
    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameMakeChan>();
        *stack_frame.result_ptr = ptr;
    };
    ctx.leave()
}

#[repr(C)]
struct StackFrameMakeClosure {
    common: StackFrameCommon,
    result_ptr: *mut FunctionObject,
    user_function: UserFunction,
    num_object_ptrs: usize,
    object_ptrs: [ObjectPtr; 0],
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

        let stack_frame = ctx.stack_frame::<StackFrameMakeClosure>();

        closure_layout.func = stack_frame.user_function.clone();

        let num_object_ptrs = stack_frame.num_object_ptrs;
        let object_ptrs = stack_frame.object_ptrs.as_ptr();
        let object_ptrs = slice::from_raw_parts(object_ptrs, num_object_ptrs).to_vec();

        let prev_object_ptrs = mem::replace(&mut closure_layout.object_ptrs, object_ptrs);
        mem::forget(prev_object_ptrs);
    };
    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameMakeClosure>();
        *stack_frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());
    };
    ctx.leave()
}

#[repr(C)]
struct StackFrameMakeInterface {
    common: StackFrameCommon,
    result_ptr: *mut Interface,
    receiver: ObjectPtr,
    type_id: TypeId,
}

pub fn make_interface(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMakeInterface>();

    let receiver = if frame.receiver.is_null() {
        ObjectPtr(ptr::null_mut())
    } else {
        let size = frame.type_id.size();
        let ptr = ctx
            .global_context()
            .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
        unsafe {
            ptr::copy_nonoverlapping(frame.receiver.0 as *const u8, ptr as *mut u8, size);
        }
        ObjectPtr(ptr)
    };

    let interface = Interface::new(receiver, frame.type_id);
    unsafe {
        ptr::copy_nonoverlapping(&interface, frame.result_ptr, 1);
    }
    mem::forget(interface);
    ctx.leave()
}

#[repr(C)]
struct StackFrameMakeStringFromByteSlice {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    byte_slice: Slice,
}

pub fn make_string_from_byte_slice(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let len = {
        let stack_frame = ctx.stack_frame::<StackFrameMakeStringFromByteSlice>();
        stack_frame.byte_slice.size
    };

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context.allocator().allocate(len + 1, |_| {}) as *mut libc::c_char
    });

    let byte_slice = {
        let stack_frame = ctx.stack_frame::<StackFrameMakeStringFromByteSlice>();
        &stack_frame.byte_slice
    };

    let dst_bytes = unsafe { slice::from_raw_parts_mut(ptr, len + 1) };
    if let Some(src_bytes) = byte_slice.as_raw_slice::<libc::c_char>() {
        dst_bytes[..len].clone_from_slice(&src_bytes[..len]);
    }
    dst_bytes[len] = 0;

    let result = unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameMakeStringFromByteSlice>();
        &mut (*stack_frame.result_ptr)
    };

    let new_string_object = StringObject(ptr);
    *result = new_string_object;

    ctx.leave()
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
        let stack_frame = ctx.stack_frame::<StackFrameMakeStringFromRune>();
        (stack_frame.rune, &mut (*stack_frame.result_ptr))
    };

    assert!(rune <= std::u32::MAX as usize);
    let chars = vec![char::from_u32(rune as u32).unwrap()];

    let new_string_object = make_string(ctx, &chars);
    *result = new_string_object;

    ctx.leave()
}

#[repr(C)]
struct StackFrameMakeStringFromRuneSlice {
    common: StackFrameCommon,
    result_ptr: *mut StringObject,
    rune_slice: Slice,
}

pub fn make_string_from_rune_slice(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let rune_slice = {
        let stack_frame = ctx.stack_frame::<StackFrameMakeStringFromRuneSlice>();
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
        let stack_frame = ctx.stack_frame_mut::<StackFrameMakeStringFromRuneSlice>();
        &mut (*stack_frame.result_ptr)
    };
    *result = new_string_object;

    ctx.leave()
}

#[repr(C)]
struct StackFrameNew {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

pub fn new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let size = {
        let stack_frame = ctx.stack_frame::<StackFrameNew>();
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
        let stack_frame = ctx.stack_frame_mut::<StackFrameNew>();
        *stack_frame.result_ptr = ptr;
    };
    ctx.leave()
}

#[repr(C)]
struct StackFrameRecv {
    common: StackFrameCommon,
    result_ptr: *mut isize,
    channel: ObjectPtr,
}

pub fn recv(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let mut channel = {
        let stack_frame = ctx.stack_frame::<StackFrameRecv>();
        stack_frame.channel.clone()
    };
    let channel = channel.as_mut::<Channel>();
    let data = channel.recv()?;
    unsafe {
        let stack_frame = ctx.stack_frame_mut::<StackFrameRecv>();
        *stack_frame.result_ptr = data;
    }
    Some(ctx.leave())
}

pub fn schedule(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.leave()
}

#[repr(C)]
struct StackFrameRunDefers {
    common: StackFrameCommon,
}

pub fn run_defers(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    if ctx.deferred_list().is_null() {
        return ctx.leave();
    }

    let deferred = unsafe { &*(ctx.deferred_list() as *const Deferred) };

    if deferred.target_stack_pointer != ctx.stack_pointer() {
        return ctx.leave();
    }

    ctx.update_deferred_list(deferred.prev_deferred);

    let current_stack_pointer = ctx.stack_pointer() as *mut StackFrame;
    let next_stack_pointer = unsafe { (current_stack_pointer as *mut StackFrameRunDefers).add(1) };
    let result_pointer = next_stack_pointer as *mut *const ();
    let next_stack_pointer = unsafe { (next_stack_pointer as *mut u8).add(deferred.result_size) };
    let next_stack_pointer = next_stack_pointer as *mut StackFrameCommon;
    let next_frame = unsafe { &mut (*next_stack_pointer) };
    next_frame.resume_func = FunctionObject::from_user_function(UserFunction::new(gox5_run_defers));
    next_frame.prev_stack_pointer = current_stack_pointer;
    next_frame.free_vars = ptr::null_mut();

    unsafe {
        let src = deferred.arg_buffer;
        let dst = (next_stack_pointer).add(1);
        let dst = dst as *mut *const ();
        let dst = if deferred.result_size > 0 {
            *(dst as *mut *mut *const ()) = result_pointer;
            dst.add(1)
        } else {
            dst
        };
        ptr::copy_nonoverlapping(src, dst, deferred.num_arg_buffer_words);
    }

    ctx.update_stack_pointer(next_stack_pointer as *mut StackFrame);

    deferred.func.clone()
}

#[repr(C)]
struct StackFrameSend {
    common: StackFrameCommon,
    channel: ObjectPtr,
    data: isize,
}

pub fn send(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let (mut channel, data) = {
        let stack_frame = ctx.stack_frame::<StackFrameSend>();
        (stack_frame.channel.clone(), stack_frame.data)
    };
    let channel = channel.as_mut::<Channel>();
    channel.send(&data)?;
    Some(ctx.leave())
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
    let new_ctx = {
        let stack_frame = ctx.stack_frame_mut::<StackFrameSpawn>();

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
    (ctx.leave(), new_ctx)
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
        let stack_frame = ctx.stack_frame::<StackFrameStrview>();
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

    ctx.leave()
}
