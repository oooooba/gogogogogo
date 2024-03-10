use std::process;

use crate::interface::Interface;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFramePanicRaise {
    common: StackFrameCommon,
    value: Interface,
}

#[no_mangle]
pub extern "C" fn gox5_panic_raise(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFramePanicRaise>();
    frame.value.panic_print();
    process::exit(1)
}

#[repr(C)]
struct StackFramePanicRecover<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
}

#[no_mangle]
pub extern "C" fn gox5_panic_recover(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFramePanicRecover>();
    *frame.result_ptr = Interface::nil();
    ctx.leave()
}
