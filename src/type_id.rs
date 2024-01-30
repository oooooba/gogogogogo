use std::slice;

use super::interface::InterfaceTableEntry;
use super::ObjectPtr;
use crate::object::string::StringObject;

#[repr(C)]
struct TypeInfo {
    name: StringObject,
    num_methods: usize,
    interface_table: *const InterfaceTableEntry,
    is_equal: extern "C" fn(ObjectPtr, ObjectPtr) -> bool,
    hash: extern "C" fn(ObjectPtr) -> usize,
    size: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(C)]
pub(crate) struct TypeId(usize);

impl TypeId {
    fn type_info(&self) -> &TypeInfo {
        unsafe { &*(self.0 as *const TypeInfo) }
    }

    pub fn interface_table(&self) -> &[InterfaceTableEntry] {
        let type_info = self.type_info();
        unsafe { slice::from_raw_parts(type_info.interface_table, type_info.num_methods) }
    }

    pub fn size(&self) -> usize {
        let type_info = self.type_info();
        type_info.size
    }

    pub fn is_equal_func(&self) -> extern "C" fn(ObjectPtr, ObjectPtr) -> bool {
        let type_info = self.type_info();
        type_info.is_equal
    }

    pub fn hash_func(&self) -> extern "C" fn(ObjectPtr) -> usize {
        let type_info = self.type_info();
        type_info.hash
    }
}
