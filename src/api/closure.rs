use std::mem;
use std::ptr;

use crate::word_chunk::WordChunk;
use crate::ClosureLayout;
use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;
use crate::UserFunction;

#[repr(C)]
struct StackFrameClosureNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut FunctionObject,
    user_function: UserFunction,
    free_vars: WordChunk,
}

#[no_mangle]
pub extern "C" fn gox5_closure_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameClosureNew>();

    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(mem::size_of::<ClosureLayout>(), |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut ClosureLayout);
            }) as *mut ClosureLayout
    });

    let wc_ptr = unsafe {
        let sp = ctx.stack_pointer() as *const u8;
        sp.add(mem::offset_of!(StackFrameClosureNew, free_vars)) as *const WordChunk
    };
    let object_ptrs = unsafe { WordChunk::as_slice_raw::<ObjectPtr>(wc_ptr) }.to_vec();
    let closure_layout = ClosureLayout::new(frame.user_function.clone(), object_ptrs);
    unsafe {
        ptr::write(ptr, closure_layout);
    }

    let frame = ctx.stack_frame_mut::<StackFrameClosureNew>();
    *frame.result_ptr = FunctionObject::from_closure_layout_ptr(ptr as *const ());

    ctx.pop_frame()
}
