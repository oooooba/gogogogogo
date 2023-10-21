use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::ptr;

use super::type_id::TypeId;
use super::{ObjectAllocator, ObjectPtr};

struct Key {
    ptr: ObjectPtr,
    _type_id: TypeId,
}

impl Key {
    fn new(ptr: ObjectPtr, type_id: TypeId) -> Self {
        Self {
            ptr,
            _type_id: type_id,
        }
    }
}

impl PartialEq for Key {
    fn eq(&self, other: &Self) -> bool {
        let lhs = self.ptr.as_ref::<usize>();
        let rhs = other.ptr.as_ref::<usize>();
        lhs == rhs
    }
}

impl Eq for Key {}

impl Hash for Key {
    fn hash<H: Hasher>(&self, state: &mut H) {
        let v = *self.ptr.as_ref::<usize>();
        v.hash(state);
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
        match self.map.get(&key) {
            Some(val) => {
                let object_size = self.value_type.size();
                unsafe {
                    ptr::copy_nonoverlapping(val.0 as *const u8, value.0 as *mut u8, object_size);
                }
                true
            }
            None => false,
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
