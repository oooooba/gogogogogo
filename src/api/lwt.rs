use crate::create_light_weight_thread_context;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::word_chunk::WordChunk;
use crate::FunctionObject;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFrameLwtSpawn {
    common: StackFrameCommon,
    func: FunctionObject,
    result_size: usize,
    args: WordChunk,
}

#[no_mangle]
pub extern "C" fn gox5_lwt_spawn(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let new_ctx = {
        let global_context = ctx.global_context().dupulicate();

        let frame = ctx.stack_frame_mut::<StackFrameLwtSpawn>();
        let entry_func = frame.func.clone();
        let result_size = frame.result_size;
        let args = frame.args.as_slice();

        let mut new_ctx = create_light_weight_thread_context(global_context, entry_func);
        let prev_stack_pointer = new_ctx.stack_pointer();
        let result_pointer = if result_size > 0 {
            Some(prev_stack_pointer as *const ())
        } else {
            None
        };
        new_ctx.grow_stack(result_size);
        new_ctx.push_frame(
            prev_stack_pointer,
            result_pointer,
            args,
            FunctionObject::from_user_function(UserFunction::new(crate::terminate)),
        );
        new_ctx
    };
    ctx.global_context().process(|mut global_context| {
        global_context.push_light_weight_thread(new_ctx);
    });
    ctx.suspend();
    ctx.pop_frame()
}

#[no_mangle]
pub extern "C" fn gox5_lwt_yield(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.pop_frame()
}
