use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::ptr;

use super::type_id::TypeId;
use super::{ObjectAllocator, ObjectPtr};

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

pub(crate) struct Map {
    map: HashMap<Key, ObjectPtr>,
    key_type: TypeId,
    value_type: TypeId,
}

impl Map {
    pub fn new(key_type: TypeId, value_type: TypeId) -> Self {
        Map {
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
}
