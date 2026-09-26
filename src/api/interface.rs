use std::mem;
use std::ptr;
use std::slice;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;
use crate::object::interface::Interface;
use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::word_chunk::WordChunk;

#[repr(C)]
struct StackFrameInterfaceNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
    receiver: ObjectPtr,
    type_id: TypeId,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_interface_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameInterfaceNew>();
    let type_id = frame.type_id;
    let frame_receiver = frame.receiver.clone();

    let receiver = if frame_receiver.is_null() {
        ObjectPtr(ptr::null_mut())
    } else {
        let size = type_id.size();
        let ptr = ctx.allocate(size, type_id);
        let src = unsafe { slice::from_raw_parts(frame_receiver.0 as *const u8, size) };
        let dst = unsafe { slice::from_raw_parts_mut(ptr as *mut u8, size) };
        dst.copy_from_slice(src);
        ObjectPtr(ptr)
    };

    let interface = Interface::new(receiver, type_id);

    let frame = ctx.stack_frame_mut::<StackFrameInterfaceNew>();
    *frame.result_ptr = interface;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameInterfaceConvertToConcreteType<'a> {
    common: StackFrameCommon,
    interface: &'a Interface,
    to_type: TypeId,
    value: ObjectPtr,
    success: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_interface_convert_to_concrete_type(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameInterfaceConvertToConcreteType>();
    let object_size = frame.to_type.size();
    let success = frame.interface.type_id() == &frame.to_type;

    let dst = unsafe { slice::from_raw_parts_mut(frame.value.as_mut() as *mut u8, object_size) };
    if success {
        let src = unsafe {
            slice::from_raw_parts(frame.interface.receiver().0 as *const u8, object_size)
        };
        dst.copy_from_slice(src);
    } else {
        if frame.success.is_null() {
            unimplemented!()
        }
        dst.fill(0);
    }

    if !frame.success.is_null() {
        *frame.success.as_mut() = success;
    }

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameInterfaceConvertToInterface<'a> {
    common: StackFrameCommon,
    interface: &'a Interface,
    to_type: TypeId,
    value: ObjectPtr,
    success: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_interface_convert_to_interface(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    let frame = ctx.stack_frame_mut::<StackFrameInterfaceConvertToInterface>();

    let success = if frame.interface.is_nil() {
        frame.to_type.interface_table().is_empty()
    } else {
        frame.to_type.interface_table().iter().all(|entry| {
            frame
                .interface
                .search_by_signature(entry.method_name(), entry.method_signature())
        })
    };

    if success {
        *frame.value.as_mut::<Interface>() = frame.interface.clone();
    } else {
        if frame.success.is_null() {
            unimplemented!()
        }
        *frame.value.as_mut::<Interface>() = Interface::nil();
    }

    if !frame.success.is_null() {
        *frame.success.as_mut() = success;
    }

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

#[unsafe(no_mangle)]
pub extern "C" fn gox5_interface_invoke(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    // The arguments live in the arg buffer of this frame, which is reused as
    // the callee's frame right below, so they must be copied out before the
    // frame is popped. Everything the frame is needed for has to be read out
    // first, because the copy allocates and may run the garbage collector,
    // which needs &mut LightWeightThreadContext.
    let (next_func, result_pointer) = {
        let frame = ctx.stack_frame::<StackFrameInterfaceInvoke>();
        let method = frame.interface.search(frame.method_name.clone());
        let result_pointer = frame.result_ptr.as_deref().map(|p| p as *const ());
        (method.unwrap(), result_pointer)
    };

    let args_wc_ptr = unsafe {
        let sp = ctx.stack_pointer() as *const u8;
        sp.add(mem::offset_of!(StackFrameInterfaceInvoke, args)) as *const WordChunk
    };
    let count = unsafe { WordChunk::count_of_raw(args_wc_ptr) };
    // This allocation must not be made through the global context directly:
    // a hot interface invocation loop can exhaust the heap, and the garbage
    // collector has to run before giving up on the allocation.
    let args = ctx.allocate(WordChunk::size_for_count(count), TypeId::new_invalid());
    let args = unsafe { WordChunk::copy_into_raw(args as *mut WordChunk, args_wc_ptr) };

    let current_stack_pointer = ctx.stack_pointer();
    let resume_func = ctx.pop_frame();
    let prev_stack_pointer = ctx.stack_pointer();
    ctx.grow_stack((current_stack_pointer as usize) - (prev_stack_pointer as usize));

    ctx.push_frame(
        prev_stack_pointer,
        result_pointer,
        unsafe { WordChunk::as_slice_raw(args.as_ptr()) },
        resume_func,
    );

    next_func
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::DeferStack;
    use crate::ObjectAllocator;
    use crate::StackFrame;
    use crate::UserFunction;
    use crate::allocator::MAX_TOTAL_ALLOCATED_SIZE;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::object::interface::InterfaceTableEntry;
    use crate::object::string::StringObject;
    use crate::type_id::{FakeTypeInfo, TypeId, TypeInfo};
    use std::mem;
    use std::slice;
    use std::sync::OnceLock;

    extern "C" fn test_is_equal(a: ObjectPtr, b: ObjectPtr) -> bool {
        unsafe { *(a.0 as *const isize) == *(b.0 as *const isize) }
    }

    extern "C" fn test_hash(a: ObjectPtr) -> usize {
        unsafe { *(a.0 as *const isize) as usize }
    }

    fn test_type_info() -> &'static TypeInfo {
        static INSTANCE: OnceLock<TypeInfo> = OnceLock::new();
        INSTANCE.get_or_init(|| {
            static TEST_NAME: [u8; 5] = *b"test\0";
            let name = StringObject::new(TEST_NAME.as_ptr(), 4);
            TypeInfo {
                name,
                num_methods: 0,
                interface_table: ptr::null(),
                is_equal: test_is_equal,
                hash: test_hash,
                size: mem::size_of::<isize>(),
                no_pointers: false,
                is_interface: false,
                get_member_offset_runs: None,
            }
        })
    }

    fn test_type_id() -> TypeId {
        TypeId::from_raw(test_type_info() as *const TypeInfo as usize)
    }

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
    fn test_gox5_interface_new_null_receiver() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<Interface>());
        let result_raw = ctx.stack_pointer() as *mut Interface;
        ctx.grow_stack(mem::size_of::<StackFrameInterfaceNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameInterfaceNew>();
        frame.receiver = ObjectPtr(ptr::null_mut());
        frame.type_id = test_type_id();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_interface_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert!(res.receiver().is_null());
        assert_eq!(*res.type_id(), test_type_id());
    }

    #[test]
    fn test_gox5_interface_new_with_receiver() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<Interface>());
        let result_raw = ctx.stack_pointer() as *mut Interface;
        ctx.grow_stack(mem::size_of::<StackFrameInterfaceNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let val: isize = 42;
        let receiver = ObjectPtr(&val as *const isize as *mut ());

        let frame = ctx.stack_frame_mut::<StackFrameInterfaceNew>();
        frame.receiver = receiver;
        frame.type_id = test_type_id();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_interface_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert!(!res.is_nil());
        assert_eq!(*res.type_id(), test_type_id());
    }

    static TEST_METHOD_NAME: [u8; 7] = *b"Method\0";
    static TEST_METHOD_SIGNATURE: [u8; 5] = *b"()()\0";

    unsafe extern "C" fn test_method(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    unsafe extern "C" fn test_invoke_resume(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    /// The single interface table entry the tests dispatch through. The method
    /// FunctionObject is built exactly once and handed out from here, because
    /// under Miri the same fn item turned into a FunctionObject at two
    /// different source sites compares unequal.
    fn test_interface_table() -> &'static [InterfaceTableEntry] {
        static INSTANCE: OnceLock<[InterfaceTableEntry; 1]> = OnceLock::new();
        INSTANCE.get_or_init(|| {
            [InterfaceTableEntry::new(
                StringObject::new(TEST_METHOD_NAME.as_ptr(), TEST_METHOD_NAME.len() - 1),
                FunctionObject::from_user_function(UserFunction::new(test_method)),
                StringObject::new(
                    TEST_METHOD_SIGNATURE.as_ptr(),
                    TEST_METHOD_SIGNATURE.len() - 1,
                ),
            )]
        })
    }

    fn test_method_object() -> FunctionObject {
        test_interface_table()[0].method().clone()
    }

    /// A TypeInfo whose interface table holds a single `Method` entry bound to
    /// `test_method_object`.
    fn test_method_type_id() -> TypeId {
        static INSTANCE: OnceLock<TypeInfo> = OnceLock::new();
        let table = test_interface_table();
        let info = INSTANCE.get_or_init(|| TypeInfo {
            name: StringObject::new(TEST_METHOD_NAME.as_ptr(), TEST_METHOD_NAME.len() - 1),
            num_methods: table.len(),
            interface_table: table.as_ptr(),
            is_equal: test_is_equal,
            hash: test_hash,
            size: mem::size_of::<isize>(),
            no_pointers: false,
            is_interface: false,
            get_member_offset_runs: None,
        });
        TypeId::from_raw(info as *const TypeInfo as usize)
    }

    fn test_interface() -> (Interface, isize) {
        let receiver: isize = 1;
        (
            Interface::new(
                ObjectPtr(&receiver as *const isize as *mut ()),
                test_method_type_id(),
            ),
            receiver,
        )
    }

    /// Builds the stack the generated C code builds for `t.Method(args...)`:
    /// a caller frame, and on top of it a StackFrameInterfaceInvoke whose
    /// argument buffer holds `args`. The context's stack pointer ends up
    /// pointing at that interface invoke frame.
    fn push_invoke_frame<'a>(
        ctx: &mut LightWeightThreadContext,
        interface: &'a Interface,
        result_ptr: Option<&'a mut ()>,
        args: &[*const ()],
    ) -> FunctionObject {
        let resume_func = FunctionObject::from_user_function(UserFunction::new(test_invoke_resume));
        let prev_stack_pointer = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>());
        ctx.push_frame(prev_stack_pointer, None, &[], resume_func.clone());
        ctx.grow_stack(mem::size_of::<StackFrameInterfaceInvoke>() + mem::size_of_val(args));

        // The argument buffer of the interface invoke frame: a count followed by
        // the argument words, exactly like the generated C code lays it out. It
        // is written through the raw stack pointer, because
        // `addr_of_mut!(frame.args)` on a `&mut StackFrameInterfaceInvoke` only
        // covers the count word, not the buffer that follows it.
        let mut raw_args: Vec<usize> = Vec::with_capacity(1 + args.len());
        raw_args.push(args.len());
        raw_args.extend(args.iter().map(|arg| *arg as usize));
        unsafe {
            let sp = ctx.stack_pointer() as *mut u8;
            let dst = sp.add(mem::offset_of!(StackFrameInterfaceInvoke, args)) as *mut WordChunk;
            WordChunk::copy_into_raw(dst, raw_args.as_ptr() as *const WordChunk);
        }

        let frame = ctx.stack_frame_mut::<StackFrameInterfaceInvoke>();
        frame.common.resume_func = resume_func.clone();
        frame.common.prev_stack_pointer = prev_stack_pointer;
        frame.common.free_vars = ptr::null_mut();
        frame.common.defer_stack = DeferStack::new();
        frame.result_ptr = result_ptr;
        frame.interface = interface;
        frame.method_name =
            StringObject::new(TEST_METHOD_NAME.as_ptr(), TEST_METHOD_NAME.len() - 1);
        resume_func
    }

    /// The argument words the callee frame pushed by gox5_interface_invoke was
    /// given, read back through the raw stack pointer for the same reason as in
    /// `push_invoke_frame`.
    fn pushed_args(ctx: &LightWeightThreadContext, num_args: usize) -> &[*const ()] {
        let sp = ctx.stack_pointer() as *const u8;
        let args = unsafe { sp.add(mem::offset_of!(StackFrame, additional_words)) };
        unsafe { slice::from_raw_parts(args as *const *const (), num_args) }
    }

    #[test]
    fn test_gox5_interface_invoke_pushes_callee_frame_with_args() {
        let (mut ctx, _gc) = create_ctx();
        let (interface, _receiver) = test_interface();
        let args: Vec<*const ()> = vec![0x1111 as *const (), 0x2222 as *const ()];
        let resume_func = push_invoke_frame(&mut ctx, &interface, None, &args);

        let result = gox5_interface_invoke(&mut ctx);
        assert_eq!(result, test_method_object());
        assert_eq!(pushed_args(&ctx, args.len()), args.as_slice());
        let frame = ctx.stack_frame::<StackFrameCommon>();
        assert_eq!(frame.resume_func, resume_func);
    }

    #[test]
    fn test_gox5_interface_invoke_passes_result_ptr() {
        let (mut ctx, _gc) = create_ctx();
        let (interface, _receiver) = test_interface();
        let mut result_slot = Box::new(());
        let result_ref: &mut () = &mut result_slot;
        let args: Vec<*const ()> = vec![];
        push_invoke_frame(&mut ctx, &interface, Some(result_ref), &args);

        gox5_interface_invoke(&mut ctx);
        let sp = ctx.stack_pointer();
        let result_ptr = unsafe {
            *sp.cast::<u8>()
                .add(mem::offset_of!(StackFrame, additional_words))
                .cast::<*const ()>()
        };
        assert_eq!(result_ptr, &*result_slot as *const ());
    }

    /// The argument buffer copy made by gox5_interface_invoke used to be
    /// allocated straight through the global context, so a hot interface
    /// invocation loop could exhaust the heap and get a null pointer back
    /// instead of running the garbage collector.
    #[test]
    fn test_gox5_interface_invoke_allocates_args_when_heap_is_exhausted() {
        let (mut ctx, gc) = create_ctx();

        // Fill the heap with one referenced chunk and one collectible chunk. The
        // fillers are declared pointer-free, which keeps the collection from
        // having to scan this megabyte of them word by word.
        let filler_type = FakeTypeInfo::new(true);
        let collectible_size = 65536;
        let referenced_size = MAX_TOTAL_ALLOCATED_SIZE - collectible_size;
        let (start, _end) = ctx.stack_range();
        ctx.grow_stack(mem::size_of::<usize>());
        let referenced = ctx.allocate(referenced_size, filler_type.tid());
        assert!(!referenced.is_null());
        unsafe { ptr::write(start as *mut usize, referenced as usize) };
        let collectible = ctx.allocate(collectible_size, filler_type.tid());
        assert!(!collectible.is_null());
        let total_before = gc.process(|mut gc| gc.allocator().total_size());

        let (interface, _receiver) = test_interface();
        let args: Vec<*const ()> = vec![0x3333 as *const ()];
        push_invoke_frame(&mut ctx, &interface, None, &args);

        let result = gox5_interface_invoke(&mut ctx);
        assert_eq!(result, test_method_object());
        assert_eq!(pushed_args(&ctx, args.len()), args.as_slice());
        assert!(
            gc.process(|mut gc| gc.allocator().total_size()) < total_before,
            "the garbage collector should have reclaimed the unreferenced chunk"
        );
    }
}
