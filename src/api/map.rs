use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::StackFrameCommon;
use crate::object::interface::Interface;
use crate::object::map::MapObject;
use crate::type_id::TypeId;

#[repr(C)]
struct StackFrameMapNew<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut ObjectPtr,
    key_type: TypeId,
    value_type: TypeId,
}

fn allocate_map(ctx: &mut LightWeightThreadContext, map: MapObject) -> *mut MapObject {
    let object_size = mem::size_of::<MapObject>();
    let ptr = ctx.global_context().process(|mut global_context| {
        global_context
            .allocator()
            .allocate(object_size, |ptr| unsafe {
                ptr::drop_in_place(ptr as *mut MapObject)
            }) as *mut MapObject
    });

    unsafe {
        ptr::write(ptr, map);
    }
    ptr
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapNew>();

    let ptr = allocate_map(ctx, MapObject::new(frame.key_type, frame.value_type));
    let ptr = ObjectPtr(ptr as *mut ());

    let frame = ctx.stack_frame_mut::<StackFrameMapNew>();
    *frame.result_ptr = ptr;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapClone<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut Interface,
    map: Interface,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_clone(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapClone>();

    if frame.map.is_nil() {
        let frame = ctx.stack_frame_mut::<StackFrameMapClone>();
        *frame.result_ptr = Interface::nil();
        return ctx.pop_frame();
    }

    let type_id = *frame.map.type_id();
    let receiver = frame.map.receiver();
    let map_ptr = unsafe { *(receiver.0 as *const *mut ()) };
    let map = unsafe { &*(map_ptr as *const MapObject) };
    let cloned = map.clone();

    let ptr = allocate_map(ctx, cloned);
    let slot_size = mem::size_of::<*mut ()>();
    let slot = ctx
        .global_context()
        .process(|mut global_context| global_context.allocator().allocate(slot_size, |_ptr| {}));
    unsafe {
        *(slot as *mut *mut ()) = ptr as *mut ();
    }
    let interface = Interface::new(ObjectPtr(slot), type_id);

    let frame = ctx.stack_frame_mut::<StackFrameMapClone>();
    *frame.result_ptr = interface;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapDelete {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_delete(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapDelete>();

    if frame.map.is_null() {
        return ctx.pop_frame();
    }

    let mut map = frame.map.clone();
    let key = frame.key.clone();
    let map = map.as_mut::<MapObject>();
    map.delete(key);

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapGet {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
    found: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_get(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapGet>();

    let found = if frame.map.is_null() {
        false
    } else {
        let map = frame.map.as_ref::<MapObject>();
        let key = frame.key.clone();
        let value = frame.value.clone();
        map.get(key, value)
    };

    if !frame.found.is_null() {
        let frame = ctx.stack_frame_mut::<StackFrameMapGet>();
        *frame.found.as_mut() = found;
    }

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapLen<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut usize,
    map: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_len(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapLen>();

    let len = if frame.map.is_null() {
        0
    } else {
        let map = frame.map.as_ref::<MapObject>();
        map.len()
    };

    let frame = ctx.stack_frame_mut::<StackFrameMapLen>();
    *frame.result_ptr = len;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapNext {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
    found: ObjectPtr,
    count: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_next(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapNext>();

    let found = if frame.map.is_null() {
        false
    } else {
        let mut map = frame.map.clone();
        let map = map.as_mut::<MapObject>();
        let key = frame.key.clone();
        let value = frame.value.clone();
        let nth = *frame.count.as_ref::<usize>();
        map.nth(key, value, nth)
    };

    let frame = ctx.stack_frame_mut::<StackFrameMapNext>();
    *frame.found.as_mut() = found;
    if found {
        let nth = *frame.count.as_ref::<usize>();
        *frame.count.as_mut() = nth + 1;
    }

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameMapSet {
    common: StackFrameCommon,
    map: ObjectPtr,
    key: ObjectPtr,
    value: ObjectPtr,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_map_set(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameMapSet>();

    if frame.map.is_null() {
        unimplemented!()
    }

    let mut map = frame.map.clone();
    let key = frame.key.clone();
    let value = frame.value.clone();

    ctx.global_context().process(|mut global_context| {
        let map = map.as_mut::<MapObject>();
        let allocator = global_context.allocator();
        map.set(key, value, allocator);
    });

    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::object::string::StringObject;
    use crate::type_id::TypeId;
    use std::mem;
    use std::ptr;
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
        interface_table: *const crate::object::interface::InterfaceTableEntry,
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

    fn make_map_ptr(allocator: &mut MockObjectAllocator, map: MapObject) -> ObjectPtr {
        let size = mem::size_of::<MapObject>();
        let ptr = allocator.allocate(size, |ptr| unsafe {
            ptr::drop_in_place(ptr as *mut MapObject);
        }) as *mut MapObject;
        unsafe {
            ptr::write(ptr, map);
        }
        ObjectPtr(ptr as *mut ())
    }

    fn make_isize_ptr(allocator: &mut MockObjectAllocator, value: isize) -> ObjectPtr {
        let ptr = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
        unsafe { *ptr = value };
        ObjectPtr(ptr as *mut ())
    }

    #[test]
    fn test_gox5_map_new() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<ObjectPtr>());
        let result_raw = ctx.stack_pointer() as *mut ObjectPtr;
        ctx.grow_stack(mem::size_of::<StackFrameMapNew>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameMapNew>();
        frame.key_type = test_type_id();
        frame.value_type = test_type_id();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_map_new(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { &*result_raw };
        assert!(!res.is_null());
        let map = res.as_ref::<MapObject>();
        assert_eq!(map.len(), 0);
    }

    #[test]
    fn test_gox5_map_set_and_len() {
        let mut allocator = MockObjectAllocator::new();
        let map = MapObject::new(test_type_id(), test_type_id());
        let map_ptr = make_map_ptr(&mut allocator, map);

        let key = make_isize_ptr(&mut allocator, 42);
        let value = make_isize_ptr(&mut allocator, 100);

        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameMapSet>());
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());

        let frame = ctx.stack_frame_mut::<StackFrameMapSet>();
        frame.map = map_ptr.clone();
        frame.key = key;
        frame.value = value;

        let result = gox5_map_set(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
    }

    #[test]
    fn test_gox5_map_len_empty() {
        let mut allocator = MockObjectAllocator::new();
        let map = MapObject::new(test_type_id(), test_type_id());
        let map_ptr = make_map_ptr(&mut allocator, map);

        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<usize>());
        let result_raw = ctx.stack_pointer() as *mut usize;
        ctx.grow_stack(mem::size_of::<StackFrameMapLen>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameMapLen>();
        frame.map = map_ptr;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_map_len(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 0);
    }

    #[test]
    fn test_gox5_map_len_null() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<usize>());
        let result_raw = ctx.stack_pointer() as *mut usize;
        ctx.grow_stack(mem::size_of::<StackFrameMapLen>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameMapLen>();
        frame.map = ObjectPtr(ptr::null_mut());
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_map_len(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let res = unsafe { *result_raw };
        assert_eq!(res, 0);
    }

    #[test]
    fn test_gox5_map_delete_null() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameMapDelete>());
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());

        let frame = ctx.stack_frame_mut::<StackFrameMapDelete>();
        frame.map = ObjectPtr(ptr::null_mut());
        frame.key = ObjectPtr(ptr::null_mut());

        let result = gox5_map_delete(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
    }
}
