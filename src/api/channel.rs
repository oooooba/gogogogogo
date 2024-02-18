use core::slice;
use std::mem;
use std::ptr;

use crate::object::channel::ChannelObject;
use crate::object::channel::ReceiveStatus;
use crate::type_id::TypeId;
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

fn allocate_channel(ctx: &mut LightWeightThreadContext, capacity: usize) -> *mut ChannelObject {
    let object_size = mem::size_of::<ChannelObject>();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(object_size, |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut ChannelObject)
            }) as *mut ChannelObject
    });

    let channel = ctx
        .global_context()
        .process(|mut global_context| ChannelObject::new(capacity, global_context.allocator()));

    unsafe {
        ptr::copy_nonoverlapping(&channel, ptr, 1);
    }

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
struct SelectEntry {
    channel: ObjectPtr,
    type_id: TypeId,
    send_data: ObjectPtr,
    receive_data: ObjectPtr,
}

#[repr(C)]
struct StackFrameChannelSelect<'a> {
    common: StackFrameCommon,
    selected_index: &'a mut isize,
    receive_available: &'a mut bool,
    entry_count: usize,
    entry_buffer: [SelectEntry; 0],
}

#[no_mangle]
pub extern "C" fn gox5_channel_select(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameChannelSelect>();
    let entry_buffer = unsafe {
        std::slice::from_raw_parts_mut(frame.entry_buffer.as_mut_ptr(), frame.entry_count)
    };
    loop {
        for (i, entry) in entry_buffer.iter_mut().enumerate() {
            let mut channel = entry.channel.clone();
            let channel = channel.as_mut::<ChannelObject>();
            let id = ctx.id();
            if !entry.send_data.is_null() {
                let data_size = entry.type_id.size();
                let data = ctx.global_context().process(|mut global_context| {
                    global_context.allocator().allocate(data_size, |_| {})
                });
                unsafe {
                    let src = slice::from_raw_parts(entry.send_data.as_ref::<u8>(), data_size);
                    let dst = slice::from_raw_parts_mut(data as *mut u8, data_size);
                    dst.copy_from_slice(src);
                };
                let data = ObjectPtr(data);
                if let Some(()) = channel.send(id, data) {
                    let frame = ctx.stack_frame_mut::<StackFrameChannelSelect>();
                    *frame.selected_index = isize::try_from(i).unwrap();
                    *frame.receive_available = false;
                    return ctx.leave();
                }
            }
            if !entry.receive_data.is_null() {
                match channel.receive(id) {
                    ReceiveStatus::Value(data) => {
                        let frame = ctx.stack_frame_mut::<StackFrameChannelSelect>();
                        *frame.selected_index = isize::try_from(i).unwrap();
                        *frame.receive_available = true;

                        let data_size = entry.type_id.size();
                        unsafe {
                            let src = slice::from_raw_parts(data.as_ref::<u8>(), data_size);
                            let dst = slice::from_raw_parts_mut(
                                entry.receive_data.as_mut::<u8>(),
                                data_size,
                            );
                            dst.copy_from_slice(src);
                        };

                        return ctx.leave();
                    }
                    ReceiveStatus::Blocked => (),
                    ReceiveStatus::Closed => (),
                };
            }
        }

        for (i, entry) in entry_buffer.iter_mut().enumerate() {
            let mut channel = entry.channel.clone();
            let channel = channel.as_mut::<ChannelObject>();
            let id = ctx.id();
            if !entry.send_data.is_null() {
                unreachable!();
            }
            if !entry.receive_data.is_null() {
                match channel.receive(id) {
                    ReceiveStatus::Value(_) => break,
                    ReceiveStatus::Blocked => (),
                    ReceiveStatus::Closed => {
                        let frame = ctx.stack_frame_mut::<StackFrameChannelSelect>();
                        *frame.selected_index = isize::try_from(i).unwrap();
                        *frame.receive_available = false;

                        let data_size = entry.type_id.size();
                        unsafe {
                            let dst = entry.receive_data.as_mut::<u8>();
                            ptr::write_bytes(dst, 0, data_size);
                        }

                        return ctx.leave();
                    }
                };
            }
        }
    }
}

#[repr(C)]
struct StackFrameChannelClose {
    common: StackFrameCommon,
    channel: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_channel_close(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameChannelClose>();
    let mut channel = frame.channel.clone();
    let channel = channel.as_mut::<ChannelObject>();
    let id = ctx.id();

    channel.close(id);

    ctx.leave()
}

#[repr(C)]
struct StackFrameChannelReceive {
    common: StackFrameCommon,
    channel: ObjectPtr,
    type_id: TypeId,
    data: ObjectPtr,
    available: ObjectPtr,
}

pub fn recv(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let frame = ctx.stack_frame::<StackFrameChannelReceive>();
    let mut channel = frame.channel.clone();

    let channel = channel.as_mut::<ChannelObject>();
    let id = ctx.id();
    let data = match channel.receive(id) {
        ReceiveStatus::Value(data) => Some(data),
        ReceiveStatus::Blocked => return None,
        ReceiveStatus::Closed => None,
    };

    let frame = ctx.stack_frame_mut::<StackFrameChannelReceive>();
    if !frame.available.is_null() {
        *frame.available.as_mut::<bool>() = data.is_some();
    }

    let data_size = frame.type_id.size();
    if let Some(data) = data {
        unsafe {
            let src = slice::from_raw_parts(data.as_ref::<u8>(), data_size);
            let dst = slice::from_raw_parts_mut(frame.data.as_mut::<u8>(), data_size);
            dst.copy_from_slice(src);
        };
    } else {
        unsafe {
            let dst = frame.data.as_mut::<u8>();
            ptr::write_bytes(dst, 0, data_size);
        }
    }

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
    data: ObjectPtr,
    type_id: TypeId,
}

pub fn send(ctx: &mut LightWeightThreadContext) -> Option<FunctionObject> {
    let frame = ctx.stack_frame::<StackFrameChannelSend>();
    let mut channel = frame.channel.clone();

    let data_size = frame.type_id.size();
    let data = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(data_size, |_| {}));
    unsafe {
        let src = slice::from_raw_parts(frame.data.as_ref::<u8>(), data_size);
        let dst = slice::from_raw_parts_mut(data as *mut u8, data_size);
        dst.copy_from_slice(src);
    };
    let data = ObjectPtr(data);

    let channel = channel.as_mut::<ChannelObject>();
    let id = ctx.id();
    channel.send(id, data)?;

    Some(ctx.leave())
}

#[no_mangle]
pub extern "C" fn gox5_channel_send(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}
