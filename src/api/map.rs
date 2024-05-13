use std::mem;
use std::ptr;

use crate::object::map::MapObject;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameMapNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut ObjectPtr,
    key_type: TypeId,
    value_type: TypeId,
}

fn allocate_map(
    ctx: &mut LightWeightThreadContext,
    key_type: TypeId,
    value_type: TypeId,
) -> *mut MapObject {
    let object_size = mem::size_of::<MapObject>();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(object_size, |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut MapObject)
            }) as *mut MapObject
    });

    let map = MapObject::new(key_type, value_type);
    unsafe {
        ptr::copy_nonoverlapping(&map, ptr, 1);
    }
    mem::forget(map);
    ptr
}

#[no_mangle]
pub extern "C" fn gox5_map_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = &ctx.stack_frame::<StackFrameMapNew>();

    let ptr = allocate_map(ctx, frame.key_type, frame.value_type);
    let ptr = ObjectPtr(ptr as *mut ());

    let frame = ctx.stack_frame_mut::<StackFrameMapNew>();
    *frame.result_ptr = ptr;

    ctx.leave()
}

#[repr(C)]
struct StackFrameMapDelete {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_map_delete(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapDelete>();

    if frame.map.is_null() {
        return ctx.leave();
    }

    let mut map = frame.map.clone();
    let key = frame.key.clone();
    let map = map.as_mut::<MapObject>();
    map.delete(key);

    ctx.leave()
}

#[repr(C)]
struct StackFrameMapGet {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
    found: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_map_get(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapGet>();

    if frame.map.is_null() {
        unimplemented!()
    };

    let map = frame.map.as_ref::<MapObject>();
    let key = frame.key.clone();
    let value = frame.value.clone();
    let found = map.get(key, value);

    if frame.found.is_null() {
        assert!(found);
    } else {
        let frame = ctx.stack_frame_mut::<StackFrameMapGet>();
        *frame.found.as_mut() = found;
    }

    ctx.leave()
}

#[repr(C)]
struct StackFrameMapLen<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut usize,
    map: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_map_len(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapLen>();

    let len = if frame.map.is_null() {
        0
    } else {
        let map = frame.map.as_ref::<MapObject>();
        map.len()
    };

    let frame = ctx.stack_frame_mut::<StackFrameMapLen>();
    *frame.result_ptr = len;

    ctx.leave()
}

#[repr(C)]
struct StackFrameMapNext {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
    found: ObjectPtr,
    count: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_map_next(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapNext>();

    if frame.map.is_null() {
        unimplemented!()
    };

    let map = frame.map.as_ref::<MapObject>();
    let key = frame.key.clone();
    let value = frame.value.clone();
    let nth = *frame.count.as_ref::<usize>();
    let found = map.nth(key, value, nth);

    let frame = ctx.stack_frame_mut::<StackFrameMapNext>();
    *frame.found.as_mut() = found;
    if found {
        *frame.count.as_mut() = nth + 1;
    }

    ctx.leave()
}

#[repr(C)]
struct StackFrameMapSet {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
}

#[no_mangle]
pub extern "C" fn gox5_map_set(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapSet>();

    if frame.map.is_null() {
        unimplemented!()
    }

    let mut map = frame.map.clone();
    let key = frame.key.clone();
    let value = frame.value.clone();

    ctx.global_context().process(|mut global_context| {
        let map = map.as_mut::<MapObject>();
        let allocator = global_context.allocator();
        map.set(key, value, allocator);
    });

    ctx.leave()
}
