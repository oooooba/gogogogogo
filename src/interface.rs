use super::api::StringObject;
use super::type_id::TypeId;
use super::FunctionObject;
use super::ObjectPtr;

#[derive(Debug)]
#[repr(C)]
pub(crate) struct InterfaceTableEntry {
    method_name: StringObject,
    method: FunctionObject,
}

#[derive(Debug)]
#[repr(C)]
pub(crate) struct Interface {
    receiver: ObjectPtr,
    type_id: TypeId,
}

impl Interface {
    pub fn new(receiver: ObjectPtr, type_id: TypeId) -> Self {
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
}
