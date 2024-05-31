use std::ptr;

use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::ObjectPtr;

#[derive(Debug)]
#[repr(C)]
pub(crate) struct InterfaceTableEntry {
    method_name: StringObject,
    method: FunctionObject,
}

#[derive(Debug, Clone)]
#[repr(C)]
pub(crate) struct Interface {
    receiver: ObjectPtr,
    type_id: TypeId,
}

impl Interface {
    pub fn new(receiver: ObjectPtr, type_id: TypeId) -> Self {
        Self { receiver, type_id }
    }

    pub fn nil() -> Self {
        let receiver = ObjectPtr(ptr::null_mut());
        let type_id = TypeId::new_invalid();
        Self { receiver, type_id }
    }

    pub fn search(&self, method_name: StringObject) -> Option<FunctionObject> {
        let table = self.type_id.interface_table();
        for entry in table {
            if entry.method_name == method_name {
                return Some(entry.method.clone());
            }
        }
        None
    }

    pub fn panic_print(&self) {
        assert!(self.receiver.is_null());
        eprintln!("panic: nil");
    }
}
