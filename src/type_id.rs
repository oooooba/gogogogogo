use std::slice;

use super::api::StringObject;
use super::interface::InterfaceTableEntry;
use super::ObjectPtr;

#[repr(C)]
struct TypeInfo {
    name: StringObject,
    num_methods: usize,
    interface_table: *const InterfaceTableEntry,
    is_equal: *const (), // ToDo: unimplemented
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

    pub fn hash_func(&self) -> extern "C" fn(ObjectPtr) -> usize {
        let type_info = self.type_info();
        type_info.hash
    }
}
