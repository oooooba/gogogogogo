use crate::create_light_weight_thread_context;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::FunctionObject;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFrameLwtSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    num_arg_buffer_words: usize,
    arg_buffer: [*const (); 0],
}

#[no_mangle]
pub extern "C" fn gox5_lwt_spawn(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let new_ctx = {
        let frame = ctx.stack_frame_mut::<StackFrameLwtSpawn>();

        let entry_func = frame.func.clone();
        let result_size = frame.result_size;
        let args = unsafe {
            std::slice::from_raw_parts(frame.arg_buffer.as_mut_ptr(), frame.num_arg_buffer_words)
        };
        let global_context = ctx.global_context().dupulicate();

        let mut new_ctx = create_light_weight_thread_context(global_context, entry_func);
        new_ctx.call::<StackFrameCommon>(
            result_size,
            args,
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );
        new_ctx
    };
    ctx.global_context().process(|mut global_context| {
        global_context.push_light_weight_thread(new_ctx);
    });
    ctx.suspend();
    ctx.leave()
}

#[no_mangle]
pub extern "C" fn gox5_lwt_yield(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.leave()
}
