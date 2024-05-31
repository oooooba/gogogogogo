use std::process;

use crate::object::interface::Interface;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFramePanicRaise {
    common: StackFrameCommon,
    value: Interface,
}

extern "C" fn panic_raise_body(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFramePanicRaise>();
    let prev_frame = frame.common.prev_stack_frame_mut::<StackFrameCommon>();
    match prev_frame.defer_stack_mut().pop() {
        Some(mut entry) => {
            let entry = unsafe { entry.as_mut() };
            ctx.call::<StackFramePanicRaise>(
                entry.result_size(),
                entry.args(),
                FunctionObject::from_user_function(UserFunction::new(panic_raise_body)),
            );
            entry.func()
        }
        None => {
            ctx.leave();
            if ctx.is_panicking() {
                if ctx.is_stack_empty() {
                    ctx.exit_panic().panic_print();
                    process::exit(1);
                }
                FunctionObject::from_user_function(UserFunction::new(panic_raise_body))
            } else {
                ctx.leave()
            }
        }
    }
}

#[no_mangle]
pub extern "C" fn gox5_panic_raise(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFramePanicRaise>();
    let data = frame.value.clone();
    ctx.enter_panic(data);
    FunctionObject::from_user_function(UserFunction::new(panic_raise_body))
}

#[repr(C)]
struct StackFramePanicRecover<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
}

#[no_mangle]
pub extern "C" fn gox5_panic_recover(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let result = if ctx.is_panicking() {
        ctx.exit_panic()
    } else {
        Interface::nil()
    };
    let frame = ctx.stack_frame_mut::<StackFramePanicRecover>();
    *frame.result_ptr = result;
    ctx.leave()
}
