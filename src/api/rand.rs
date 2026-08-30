use std::sync::atomic::{AtomicU64, Ordering};

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;

#[repr(C)]
struct StackFrameRuntimeRand<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut u64,
}

/// math/rand.runtime_rand (`//go:linkname runtime_rand runtime.rand`) has no Go
/// body. Route calls to it into the runtime, returning a deterministic
/// pseudo-random sequence (a fixed-seed LCG) so that programs using the global
/// rand source make progress and don't all draw the same value.
#[unsafe(no_mangle)]
pub extern "C" fn gox5_runtime_rand(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    static STATE: AtomicU64 = AtomicU64::new(0x5851f42d4c957f2d);
    let mut state = STATE.load(Ordering::Relaxed);
    state = state
        .wrapping_mul(6364136223846793005)
        .wrapping_add(1442695040888963407);
    STATE.store(state, Ordering::Relaxed);

    let frame = ctx.stack_frame_mut::<StackFrameRuntimeRand>();
    *frame.result_ptr = state;
    ctx.pop_frame()
}
