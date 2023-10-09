use std::slice;

use super::api::FunctionObject;
use super::api::StringObject;
use super::type_id::TypeId;
use super::ObjectPtr;

#[derive(Debug)]
#[repr(C)]
struct InterfaceTableEntry {
    method_name: StringObject,
    method: FunctionObject,
}

#[derive(Debug)]
#[repr(C)]
pub(crate) struct Interface {
    receiver: ObjectPtr,
    type_id: TypeId,
    num_methods: usize,
    interface_table: *const InterfaceTableEntry,
}

impl Interface {
    pub fn new(
        receiver: ObjectPtr,
        type_id: TypeId,
        num_methods: usize,
        interface_table: *const (),
    ) -> Self {
        let interface_table = interface_table as *const InterfaceTableEntry;
        Self {
            receiver,
            type_id,
            num_methods,
            interface_table,
        }
    }

    pub fn receiver(&self) -> ObjectPtr {
        self.receiver.clone()
    }

    pub fn search(&self, method_name: StringObject) -> Option<FunctionObject> {
        let table = unsafe { slice::from_raw_parts(self.interface_table, self.num_methods) };
        for entry in table {
            if entry.method_name == method_name {
                return Some(entry.method.clone());
            }
        }
        None
    }
}
