use std::mem;
use std::ptr;

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

    if success {
        unsafe {
            ptr::copy_nonoverlapping(
                frame.interface.receiver().as_ref() as *const u8,
                frame.value.as_mut() as *mut u8,
                object_size,
            );
        }
    } else {
        if frame.success.is_null() {
            unimplemented!()
        }

        unsafe {
            ptr::write_bytes(frame.value.as_mut() as *mut u8, 0, object_size);
        }
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

    let success = frame.to_type.interface_table().iter().all(|entry| {
        frame
            .interface
            .search(entry.method_name().clone())
            .is_some()
    });

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
    let frame = ctx.stack_frame::<StackFrameInterfaceInvoke>();
    let method = frame.interface.search(frame.method_name.clone());
    let next_func = method.unwrap();

    let args_wc_ptr = unsafe {
        let sp = ctx.stack_pointer() as *const u8;
        sp.add(mem::offset_of!(StackFrameInterfaceInvoke, args)) as *const WordChunk
    };
    let args = ctx.global_context().process(|mut global_context| {
        let allocator = global_context.allocator();
        unsafe { WordChunk::duplicate_raw(args_wc_ptr, allocator) }
    });

    let result_pointer = frame.result_ptr.as_ref().map(|p| (*p) as *const ());

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
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::object::interface::InterfaceTableEntry;
    use crate::object::string::StringObject;
    use crate::type_id::TypeId;
    use std::mem;
    use std::sync::OnceLock;

    extern "C" fn test_is_equal(a: ObjectPtr, b: ObjectPtr) -> bool {
        unsafe { *(a.0 as *const isize) == *(b.0 as *const isize) }
    }

    extern "C" fn test_hash(a: ObjectPtr) -> usize {
        unsafe { *(a.0 as *const isize) as usize }
    }

    #[repr(C)]
    struct TestTypeInfo {
        name: StringObject,
        num_methods: usize,
        interface_table: *const InterfaceTableEntry,
        is_equal: extern "C" fn(ObjectPtr, ObjectPtr) -> bool,
        hash: extern "C" fn(ObjectPtr) -> usize,
        size: usize,
    }

    unsafe impl Send for TestTypeInfo {}
    unsafe impl Sync for TestTypeInfo {}

    fn test_type_info() -> &'static TestTypeInfo {
        static INSTANCE: OnceLock<TestTypeInfo> = OnceLock::new();
        INSTANCE.get_or_init(|| {
            static TEST_NAME: [u8; 5] = *b"test\0";
            let name: StringObject = unsafe { mem::transmute(TEST_NAME.as_ptr()) };
            TestTypeInfo {
                name,
                num_methods: 0,
                interface_table: ptr::null(),
                is_equal: test_is_equal,
                hash: test_hash,
                size: mem::size_of::<isize>(),
            }
        })
    }

    fn test_type_id() -> TypeId {
        TypeId::from_raw(test_type_info() as *const TestTypeInfo as usize)
    }

    struct AllocatedObject {
        ptr: *mut (),
        size: usize,
        destructor: fn(*mut ()),
    }

    struct MockObjectAllocator {
        allocated_objects: Vec<AllocatedObject>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated_objects: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = size.div_ceil(alignment) * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
            self.allocate(num_pages * 4096, |_| {})
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for obj in &self.allocated_objects {
                (obj.destructor)(obj.ptr);
                unsafe {
                    Vec::from_raw_parts(obj.ptr as *mut isize, 0, obj.size);
                }
            }
        }
    }

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let allocator = Box::new(MockObjectAllocator::new());
        let gc = global_context::create_global_context(allocator);
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
}
