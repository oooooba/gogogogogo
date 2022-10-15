use std::mem;
use std::mem::ManuallyDrop;
use std::ptr;
use std::slice;

use super::channel::Channel;
use super::create_light_weight_thread_context;
use super::start_light_weight_thread;
use super::LightWeightThreadContext;
use super::ObjectPtr;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct NextUserFunctionType(*mut ());

impl NextUserFunctionType {
    pub fn new_null() -> Self {
        NextUserFunctionType(ptr::null_mut())
    }
}

unsafe impl Send for NextUserFunctionType {}

type UserFunctionTypeInner =
    unsafe extern "C" fn(&mut LightWeightThreadContext) -> NextUserFunctionType;

#[derive(Clone)]
#[repr(C)]
pub struct UserFunctionType(UserFunctionTypeInner);

impl UserFunctionType {
    pub fn new(user_function: UserFunctionTypeInner) -> Self {
        UserFunctionType(user_function)
    }

    pub fn invoke(&self, ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
        unsafe { self.0(ctx) }
    }
}

impl PartialEq<UserFunctionTypeInner> for UserFunctionType {
    fn eq(&self, other: &UserFunctionTypeInner) -> bool {
        let lhs = self.0 as *const ();
        let rhs = *other as *const ();
        lhs == rhs
    }
}

unsafe impl Send for UserFunctionType {}

#[repr(C)]
struct StackFrameCommon {
    resume_func: NextUserFunctionType,
    prev_stack_pointer: *mut StackFrame,
    free_vars: *mut (),
}

unsafe impl Send for StackFrameCommon {}

#[repr(C)]
struct Slice {
    addr: *mut (),
    size: usize,
    capacity: usize,
}

impl Slice {
    fn as_raw_slice(&self) -> Option<&mut [isize]> {
        if self.capacity == 0 {
            return None;
        }
        assert_ne!(self.addr, ptr::null_mut());
        unsafe {
            Some(slice::from_raw_parts_mut(
                self.addr as *mut isize,
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

pub fn append(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    let (base, elements, result) = unsafe {
        let (base_ptr, elements_ptr, result_ptr) = {
            let stack_frame = &mut ctx.stack_pointer.append;
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
        let ptr = ctx.global_context.process(|mut global_context| {
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

    if let Some(base_raw_slice) = base.as_raw_slice() {
        if !has_space {
            let result_raw_slice = result.as_raw_slice().unwrap();
            result_raw_slice[..base_raw_slice.len()]
                .clone_from_slice(&base_raw_slice[..base_raw_slice.len()]);
        }
    }

    if let Some(elements_raw_slice) = elements.as_raw_slice() {
        let result_raw_slice = result.as_raw_slice().unwrap();
        for i in 0..elements_raw_slice.len() {
            result_raw_slice[base.size + i] = elements_raw_slice[i];
        }
    }

    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeChan {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

unsafe impl Send for StackFrameMakeChan {}

/// temporarily, exported for unit test
pub fn allocate_channel(ctx: &mut LightWeightThreadContext, capacity: usize) -> *mut Channel {
    let object_size = mem::size_of::<Channel>();
    let ptr = ctx.global_context.process(|mut global_context| {
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

pub fn make_chan(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    let size = unsafe {
        let stack_frame = &mut ctx.stack_pointer.make_chan;
        stack_frame.size
    };
    let ptr = allocate_channel(ctx, size);
    let ptr = ObjectPtr(ptr as *mut ());
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.make_chan;
        *stack_frame.result_ptr = ptr;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameMakeClosure {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    func: UserFunctionType,
    size: usize,
    var: ObjectPtr,
}

unsafe impl Send for StackFrameMakeClosure {}

#[repr(C)]
struct ClosureLayout {
    func: UserFunctionType,
    object_ptrs: [ObjectPtr; 0],
}

pub fn make_closure(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    let size = unsafe {
        let stack_frame = &mut ctx.stack_pointer.make_closure;
        stack_frame.size
    };
    let ptr = ctx.global_context.process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>() + size, |_ptr| {})
    });
    unsafe {
        assert_eq!(size, 8);
        let ptr = ptr as *mut ClosureLayout;
        let closure_layout = &mut *ptr;

        let stack_frame = &mut ctx.stack_pointer.make_closure;

        closure_layout.func = stack_frame.func.clone();

        let object_ptrs = slice::from_raw_parts_mut(closure_layout.object_ptrs.as_mut_ptr(), 1);
        object_ptrs[0] = stack_frame.var.clone();
    };
    let ptr = ObjectPtr(ptr);
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.make_closure;
        *stack_frame.result_ptr = ptr;
    };
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameNew {
    common: StackFrameCommon,
    result_ptr: *mut ObjectPtr,
    size: usize,
}

pub fn new(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    let size = unsafe {
        let stack_frame = &mut ctx.stack_pointer.new;
        stack_frame.size
    };
    let ptr = ctx
        .global_context
        .process(|mut global_context| global_context.allocator().allocate(size, |_ptr| {}));
    unsafe {
        ptr::write_bytes(ptr as *mut u8, 0, size);
    }
    let ptr = ObjectPtr(ptr);
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.new;
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

unsafe impl Send for StackFrameRecv {}

pub async fn recv(ctx: &mut LightWeightThreadContext<'_>) -> NextUserFunctionType {
    let channel = unsafe {
        let stack_frame = &mut ctx.stack_pointer.recv;
        stack_frame.channel.clone()
    };
    let channel = channel.as_ref::<Channel>();
    let data = channel.recv().await;
    let data = data.unwrap();
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.recv;
        *stack_frame.result_ptr = data;
    }
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameSend {
    common: StackFrameCommon,
    channel: ObjectPtr,
    data: isize,
}

unsafe impl Send for StackFrameSend {}

pub async fn send(ctx: &mut LightWeightThreadContext<'_>) -> NextUserFunctionType {
    let (channel, data) = unsafe {
        let stack_frame = &mut ctx.stack_pointer.send;
        (stack_frame.channel.clone(), stack_frame.data)
    };
    let channel = channel.as_ref::<Channel>();
    channel.send(data).await;
    leave_runtime_api(ctx)
}

#[repr(C)]
struct StackFrameSpawn {
    common: StackFrameCommon,
    func: UserFunctionType,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [(); 0],
}

unsafe impl Send for StackFrameSpawn {}

pub async fn spawn(ctx: &mut LightWeightThreadContext<'_>) -> NextUserFunctionType {
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.spawn;

        let entry_func = stack_frame.func.clone();
        let result_size = stack_frame.result_size;
        let arg_buffer_ptr = ObjectPtr(stack_frame.arg_buffer.as_mut_ptr());
        let num_arg_buffer_words = stack_frame.num_arg_buffer_words;
        let global_context = ctx.global_context.dupulicate();

        tokio::spawn(async move {
            let mut new_ctx = Box::new(create_light_weight_thread_context(global_context));
            start_light_weight_thread(
                entry_func,
                &mut new_ctx,
                result_size,
                arg_buffer_ptr,
                num_arg_buffer_words,
            )
            .await;
        });
    }
    leave_runtime_api(ctx)
}

fn leave_runtime_api(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.common;
        ctx.stack_pointer = &mut *stack_frame.prev_stack_pointer;
        stack_frame.resume_func.clone()
    }
}

#[repr(C)]
pub union StackFrame {
    pub words: [*mut (); 0],
    common: ManuallyDrop<StackFrameCommon>,
    append: ManuallyDrop<StackFrameAppend>,
    make_chan: ManuallyDrop<StackFrameMakeChan>,
    make_closure: ManuallyDrop<StackFrameMakeClosure>,
    new: ManuallyDrop<StackFrameNew>,
    recv: ManuallyDrop<StackFrameRecv>,
    send: ManuallyDrop<StackFrameSend>,
    spawn: ManuallyDrop<StackFrameSpawn>,
}

unsafe impl Send for StackFrame {}
