use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::ptr;

use crate::type_id::TypeId;
use crate::{ObjectAllocator, ObjectPtr};

struct Key {
    ptr: ObjectPtr,
    type_id: TypeId,
}

impl Key {
    fn new(ptr: ObjectPtr, type_id: TypeId) -> Self {
        Self { ptr, type_id }
    }
}

impl PartialEq for Key {
    fn eq(&self, other: &Self) -> bool {
        let is_equal_func = self.type_id.is_equal_func();
        is_equal_func(self.ptr.clone(), other.ptr.clone())
    }
}

impl Eq for Key {}

impl Hash for Key {
    fn hash<H: Hasher>(&self, state: &mut H) {
        let hash_func = self.type_id.hash_func();
        let h = hash_func(self.ptr.clone());
        h.hash(state); // ToDo: use h as hash value
    }
}

pub(crate) struct MapObject {
    map: HashMap<Key, ObjectPtr>,
    key_type: TypeId,
    value_type: TypeId,
}

impl MapObject {
    pub fn new(key_type: TypeId, value_type: TypeId) -> Self {
        MapObject {
            map: HashMap::new(),
            key_type,
            value_type,
        }
    }

    pub fn len(&self) -> usize {
        self.map.len()
    }

    pub fn get(&self, key: ObjectPtr, value: ObjectPtr) -> bool {
        let key = Key::new(key, self.key_type);
        let object_size = self.value_type.size();
        match self.map.get(&key) {
            Some(val) => {
                unsafe {
                    ptr::copy_nonoverlapping(val.0 as *const u8, value.0 as *mut u8, object_size);
                }
                true
            }
            None => {
                unsafe {
                    ptr::write_bytes(value.0 as *mut u8, 0, object_size);
                }
                false
            }
        }
    }

    pub fn set(&mut self, key: ObjectPtr, value: ObjectPtr, allocator: &mut dyn ObjectAllocator) {
        let key_object_size = self.key_type.size();
        let key_ptr = allocator.allocate(key_object_size, |_| {}) as *mut u8;
        unsafe {
            ptr::copy_nonoverlapping(key.0 as *const u8, key_ptr, key_object_size);
        }
        let key = ObjectPtr(key_ptr as *mut ());

        let value_object_size = self.value_type.size();
        let value_ptr = allocator.allocate(value_object_size, |_| {}) as *mut u8;
        unsafe {
            ptr::copy_nonoverlapping(value.0 as *const u8, value_ptr, value_object_size);
        }
        let value = ObjectPtr(value_ptr as *mut ());

        let key = Key::new(key, self.key_type);
        self.map.insert(key, value);
    }

    pub fn delete(&mut self, key: ObjectPtr) {
        let key = Key::new(key, self.key_type);
        self.map.remove(&key);
    }

    pub fn nth(&self, key: ObjectPtr, value: ObjectPtr, nth: usize) -> bool {
        match self.map.iter().nth(nth) {
            Some((k, v)) => {
                if !key.is_null() {
                    unsafe {
                        let object_size = self.key_type.size();
                        ptr::copy_nonoverlapping(
                            k.ptr.0 as *const u8,
                            key.0 as *mut u8,
                            object_size,
                        );
                    }
                }
                if !value.is_null() {
                    unsafe {
                        let object_size = self.value_type.size();
                        ptr::copy_nonoverlapping(v.0 as *const u8, value.0 as *mut u8, object_size);
                    }
                }
                true
            }
            None => false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
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
        name: crate::object::string::StringObject,
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
            let name: crate::object::string::StringObject =
                unsafe { mem::transmute(TEST_NAME.as_ptr()) };
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

        fn allocate_guarded_pages(&mut self, _num_pages: usize) -> *mut () {
            unimplemented!()
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for allocated_object in &self.allocated_objects {
                (allocated_object.destructor)(allocated_object.ptr);
                unsafe {
                    Vec::from_raw_parts(
                        allocated_object.ptr as *mut isize,
                        0,
                        allocated_object.size,
                    );
                }
            }
        }
    }

    fn make_isize_ptr(allocator: &mut MockObjectAllocator, value: isize) -> ObjectPtr {
        let ptr = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
        unsafe { *ptr = value };
        ObjectPtr(ptr as *mut ())
    }

    fn make_result_ptr(allocator: &mut MockObjectAllocator) -> ObjectPtr {
        let ptr = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
        ObjectPtr(ptr as *mut ())
    }

    #[test]
    fn test_map_new() {
        let map = MapObject::new(test_type_id(), test_type_id());
        assert_eq!(map.len(), 0);
    }

    #[test]
    fn test_map_set_get() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let value = make_isize_ptr(&mut allocator, 100);
        map.set(key.clone(), value, &mut allocator);
        assert_eq!(map.len(), 1);
        let result = make_result_ptr(&mut allocator);
        assert!(map.get(key.clone(), result.clone()));
        assert_eq!(*result.as_ref::<isize>(), 100);
    }

    #[test]
    fn test_map_get_nonexistent() {
        let mut allocator = MockObjectAllocator::new();
        let map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let result = make_result_ptr(&mut allocator);
        assert!(!map.get(key, result));
    }

    #[test]
    fn test_map_set_overwrite() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let value1 = make_isize_ptr(&mut allocator, 100);
        map.set(key.clone(), value1, &mut allocator);
        let value2 = make_isize_ptr(&mut allocator, 200);
        map.set(key.clone(), value2, &mut allocator);
        assert_eq!(map.len(), 1);
        let result = make_result_ptr(&mut allocator);
        assert!(map.get(key, result.clone()));
        assert_eq!(*result.as_ref::<isize>(), 200);
    }

    #[test]
    fn test_map_delete() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let value = make_isize_ptr(&mut allocator, 100);
        map.set(key.clone(), value, &mut allocator);
        assert_eq!(map.len(), 1);
        map.delete(key.clone());
        assert_eq!(map.len(), 0);
        let result = make_result_ptr(&mut allocator);
        assert!(!map.get(key, result));
    }

    #[test]
    fn test_map_delete_nonexistent() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 999);
        map.delete(key);
        assert_eq!(map.len(), 0);
    }

    #[test]
    fn test_map_nth() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key1 = make_isize_ptr(&mut allocator, 1);
        let value1 = make_isize_ptr(&mut allocator, 10);
        map.set(key1, value1, &mut allocator);
        let key2 = make_isize_ptr(&mut allocator, 2);
        let value2 = make_isize_ptr(&mut allocator, 20);
        map.set(key2, value2, &mut allocator);
        let out_key = make_result_ptr(&mut allocator);
        let out_value = make_result_ptr(&mut allocator);
        assert!(map.nth(out_key.clone(), out_value.clone(), 0));
        assert!(map.nth(out_key.clone(), out_value.clone(), 1));
        assert!(!map.nth(out_key, out_value, 2));
    }

    #[test]
    fn test_map_nth_null_key() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let value = make_isize_ptr(&mut allocator, 10);
        map.set(key, value, &mut allocator);
        let out_value = make_result_ptr(&mut allocator);
        assert!(map.nth(ObjectPtr(ptr::null_mut()), out_value, 0));
    }

    #[test]
    fn test_map_nth_null_value() {
        let mut allocator = MockObjectAllocator::new();
        let mut map = MapObject::new(test_type_id(), test_type_id());
        let key = make_isize_ptr(&mut allocator, 1);
        let value = make_isize_ptr(&mut allocator, 10);
        map.set(key, value, &mut allocator);
        let out_key = make_result_ptr(&mut allocator);
        assert!(map.nth(out_key, ObjectPtr(ptr::null_mut()), 0));
    }
}
