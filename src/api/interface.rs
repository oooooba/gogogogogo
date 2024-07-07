use std::ptr;

use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::word_chunk::WordChunk;
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

#[repr(C)]
struct StackFrameInterfaceInvoke<'a> {
    common: StackFrameCommon,
    result_ptr: Option<&'a mut ()>,
    interface: &'a Interface,
    method_name: StringObject,
    args: WordChunk,
}

#[no_mangle]
pub extern "C" fn gox5_interface_invoke(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameInterfaceInvoke>();
    let method = frame.interface.search(frame.method_name.clone());
    let next_func = method.unwrap();

    let args = ctx.global_context().process(|mut global_context| {
        let allocator = global_context.allocator();
        frame.args.duplicate(allocator)
    });

    let result_pointer = frame.result_ptr.as_ref().map(|p| (*p) as *const ());

    let current_stack_pointer = ctx.stack_pointer();
    let resume_func = ctx.pop_frame();
    let prev_stack_pointer = ctx.stack_pointer();
    ctx.grow_stack((current_stack_pointer as usize) - (prev_stack_pointer as usize));

    ctx.push_frame(
        prev_stack_pointer,
        result_pointer,
        unsafe { args.as_ref().as_slice() },
        resume_func,
    );

    next_func
}
