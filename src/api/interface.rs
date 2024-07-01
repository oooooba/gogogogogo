use std::ptr;

use crate::object::interface::Interface;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameInterfaceNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
    receiver: ObjectPtr,
    type_id: TypeId,
}

#[no_mangle]
pub extern "C" fn gox5_interface_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameInterfaceNew>();

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

    let frame = ctx.stack_frame_mut::<StackFrameInterfaceNew>();
    *frame.result_ptr = interface;

    ctx.pop_frame()
}
