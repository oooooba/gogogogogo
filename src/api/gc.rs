use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::type_id::TypeId;

#[unsafe(no_mangle)]
pub extern "C" fn gox5_gc_register_global_object(
    ctx: &mut LightWeightThreadContext,
    address: *mut (),
    type_id: TypeId,
    size: usize,
) {
    ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .register_global_object(address, size, type_id);
    });
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_gc_collect(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.run_gc();
    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::FunctionObject;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::type_id::FakeTypeInfo;
    use std::sync::OnceLock;

    fn pointer_type_id() -> TypeId {
        static INSTANCE: OnceLock<FakeTypeInfo> = OnceLock::new();
        INSTANCE
            .get_or_init(|| FakeTypeInfo::pointer(TypeId::new_invalid()))
            .tid()
    }

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let gc = global_context::create_global_context();
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_gox5_gc_register_global_object() {
        let (mut ctx, _gc) = create_ctx();
        let mut data = [0u8; 24];
        let address = data.as_mut_ptr() as *mut ();
        gox5_gc_register_global_object(&mut ctx, address, TypeId::new_invalid(), data.len());
        let len = ctx
            .global_context()
            .process(|mut global_context| global_context.allocator().global_spans_len());
        assert_eq!(len, 1);
    }

    #[test]
    fn test_gox5_gc_collect_pops_the_runtime_frame() {
        let (mut ctx, _gc) = create_ctx();
        let prev_stack_pointer = ctx.stack_pointer();
        ctx.grow_stack(128);
        let resume = FunctionObject::new_null();
        ctx.push_frame(128, prev_stack_pointer, None, &[], resume.clone());
        let returned = gox5_gc_collect(&mut ctx);
        assert_eq!(returned, resume);
        assert_eq!(ctx.stack_pointer(), prev_stack_pointer);
    }

    #[test]
    fn test_gox5_gc_collect_keeps_reachable_and_frees_garbage() {
        let (mut ctx, _gc) = create_ctx();
        let live = ctx.allocate(16, TypeId::new_invalid());
        let garbage = ctx.allocate(16, TypeId::new_invalid());
        assert!(
            ctx.global_context()
                .process(|mut gc| gc.allocator().contains(live))
        );
        assert!(
            ctx.global_context()
                .process(|mut gc| gc.allocator().contains(garbage))
        );

        // A typed global root holds a pointer to `live`, so tracing the root by
        // its type reaches it and only `garbage` is swept.
        let mut root = [live as usize, 0usize];
        gox5_gc_register_global_object(
            &mut ctx,
            root.as_mut_ptr() as *mut (),
            pointer_type_id(),
            std::mem::size_of_val(&root),
        );
        ctx.run_gc();

        assert!(
            ctx.global_context()
                .process(|mut gc| gc.allocator().contains(live)),
            "reachable object was collected"
        );
        assert!(
            !ctx.global_context()
                .process(|mut gc| gc.allocator().contains(garbage)),
            "unreachable object survived the collection"
        );
    }
}
