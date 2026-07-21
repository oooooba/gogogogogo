use std::ptr;

use crate::object::string::StringObject;
use crate::type_id::TypeId;
use crate::FunctionObject;
use crate::ObjectPtr;

static PANIC_NIL_ERROR_DUMMY: u8 = 0;
static PANIC_NIL_ERROR_TYPE_ID_DUMMY: u8 = 0;

#[derive(Debug)]
#[repr(C)]
pub(crate) struct InterfaceTableEntry {
    method_name: StringObject,
    method: FunctionObject,
}

impl InterfaceTableEntry {
    pub(crate) fn method_name(&self) -> &StringObject {
        &self.method_name
    }
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

    pub fn panic_nil_error() -> Self {
        let receiver = ObjectPtr(&PANIC_NIL_ERROR_DUMMY as *const u8 as *mut ());
        let type_id = TypeId::from_raw(&PANIC_NIL_ERROR_TYPE_ID_DUMMY as *const u8 as usize);
        Self { receiver, type_id }
    }

    pub fn is_nil(&self) -> bool {
        self.receiver.is_null() && self.type_id == TypeId::new_invalid()
    }

    pub fn is_panic_nil_error(&self) -> bool {
        self.type_id == TypeId::from_raw(&PANIC_NIL_ERROR_TYPE_ID_DUMMY as *const u8 as usize)
    }

    pub fn receiver(&self) -> &ObjectPtr {
        &self.receiver
    }

    pub fn type_id(&self) -> &TypeId {
        &self.type_id
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
        if self.is_panic_nil_error() {
            eprintln!("panic: panic called with nil argument");
        } else if self.receiver.is_null() {
            eprintln!("panic: nil");
        } else {
            eprintln!("panic: {:?}", self);
        }
    }
}
