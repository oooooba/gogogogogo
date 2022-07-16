use std::mem;
use std::mem::ManuallyDrop;
use std::ptr;

use super::channel::Channel;
use super::create_light_weight_thread_context;
use super::start_light_weight_thread;
use super::LightWeightThreadContext;
use super::ObjectPtr;

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
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

#[derive(Clone, Copy)]
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
}

unsafe impl Send for StackFrameCommon {}

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
    arg0: *mut (),
    arg1: *mut (),
    arg2: *mut (),
}

unsafe impl Send for StackFrameSpawn {}

pub async fn spawn(ctx: &mut LightWeightThreadContext<'_>) -> NextUserFunctionType {
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.spawn;

        let entry_func = stack_frame.func;
        let args = vec![
            ObjectPtr(stack_frame.arg0),
            ObjectPtr(stack_frame.arg1),
            ObjectPtr(stack_frame.arg2),
        ];
        let global_context = ctx.global_context.dupulicate();

        tokio::spawn(async move {
            let mut new_ctx = Box::new(create_light_weight_thread_context(global_context));
            start_light_weight_thread(entry_func, &mut new_ctx, args).await;
        });
    }
    leave_runtime_api(ctx)
}

fn leave_runtime_api(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unsafe {
        let stack_frame = &mut ctx.stack_pointer.common;
        ctx.stack_pointer = &mut *stack_frame.prev_stack_pointer;
        stack_frame.resume_func
    }
}

#[repr(C)]
pub union StackFrame {
    pub words: [*mut (); 0],
    common: ManuallyDrop<StackFrameCommon>,
    make_chan: ManuallyDrop<StackFrameMakeChan>,
    new: ManuallyDrop<StackFrameNew>,
    recv: ManuallyDrop<StackFrameRecv>,
    send: ManuallyDrop<StackFrameSend>,
    spawn: ManuallyDrop<StackFrameSpawn>,
}

unsafe impl Send for StackFrame {}
