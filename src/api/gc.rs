use crate::LightWeightThreadContext;

#[unsafe(no_mangle)]
pub extern "C" fn gox5_gc_register_global_object(
    ctx: &mut LightWeightThreadContext,
    address: *mut (),
    size: usize,
) {
    ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .register_global_object(address, size);
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::FunctionObject;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let gc = global_context::create_global_context(ObjectAllocator::new());
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_gox5_gc_register_global_object() {
        let (mut ctx, _gc) = create_ctx();
        let mut data = [0u8; 24];
        let address = data.as_mut_ptr() as *mut ();
        gox5_gc_register_global_object(&mut ctx, address, data.len());
        let len = ctx
            .global_context()
            .process(|mut global_context| global_context.allocator().global_spans_len());
        assert_eq!(len, 1);
    }
}
