use std::mem;
use std::ptr;

use crate::object::channel::Channel;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

use super::ObjectPtr;

#[repr(C)]
struct StackFrameChannelNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut ObjectPtr,
    capacity: usize,
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

#[no_mangle]
pub extern "C" fn gox5_channel_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameChannelNew>();
    let capacity = frame.capacity;

    let ptr = allocate_channel(ctx, capacity);
    let result = ObjectPtr(ptr as *mut ());

    let frame = ctx.stack_frame_mut::<StackFrameChannelNew>();
    *frame.result_ptr = result;

    ctx.leave()
}

#[repr(C)]
struct StackFrameChannelReceive<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut isize,
    channel: ObjectPtr,
}

pub fn recv(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let frame = ctx.stack_frame::<StackFrameChannelReceive>();
    let mut channel = frame.channel.clone();

    let channel = channel.as_mut::<Channel>();
    let data = channel.recv()?;

    let frame = ctx.stack_frame_mut::<StackFrameChannelReceive>();
    *frame.result_ptr = data;

    Some(ctx.leave())
}

#[no_mangle]
pub extern "C" fn gox5_channel_receive(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[repr(C)]
struct StackFrameChannelSend {
    common: StackFrameCommon,
    channel: ObjectPtr,
    data: isize,
}

pub fn send(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let frame = ctx.stack_frame::<StackFrameChannelSend>();
    let mut channel = frame.channel.clone();

    let channel = channel.as_mut::<Channel>();
    channel.send(&frame.data)?;

    Some(ctx.leave())
}

#[no_mangle]
pub extern "C" fn gox5_channel_send(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}
