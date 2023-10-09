use std::slice;

use super::api::StringObject;
use super::interface::InterfaceTableEntry;

#[repr(C)]
struct TypeInfo {
    name: StringObject,
    num_methods: usize,
    interface_table: *const InterfaceTableEntry,
    is_equal: *const (), // ToDo: unimplemented
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(C)]
pub(crate) struct TypeId(usize);

impl TypeId {
    pub fn interface_table(&self) -> &[InterfaceTableEntry] {
        let type_info = unsafe { &*(self.0 as *const TypeInfo) };
        unsafe { slice::from_raw_parts(type_info.interface_table, type_info.num_methods) }
    }
}
