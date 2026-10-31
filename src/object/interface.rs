use std::mem;
use std::ptr;
use std::sync::OnceLock;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::ObjectPtr;
use crate::object::string::StringObject;
use crate::type_id::{TYPE_INFO_MAGIC, TypeId, TypeInfo, TypeKind};

static PANIC_NIL_ERROR_DUMMY: u8 = 0;
static PANIC_NIL_ERROR_TYPE_ID_DUMMY: u8 = 0;

#[repr(C)]
struct RuntimeError {
    message: StringObject,
}

extern "C" fn runtime_error_is_equal(lhs: ObjectPtr, rhs: ObjectPtr) -> bool {
    let lhs = unsafe { &*(lhs.0 as *const RuntimeError) };
    let rhs = unsafe { &*(rhs.0 as *const RuntimeError) };
    lhs.message == rhs.message
}

extern "C" fn runtime_error_hash(object: ObjectPtr) -> usize {
    let object = unsafe { &*(object.0 as *const RuntimeError) };
    let mut hash = usize::from_str_radix("cbf29ce484222325", 16).unwrap_or(0);
    for byte in object.message.as_bytes() {
        hash ^= usize::from(*byte);
        hash = hash.wrapping_mul(usize::from_str_radix("100000001b3", 16).unwrap_or(1));
    }
    hash
}

pub(crate) fn runtime_error_type_id() -> TypeId {
    static INSTANCE: OnceLock<TypeInfo> = OnceLock::new();
    static NAME: [u8; 13] = *b"runtime error";
    TypeId::from_ptr(INSTANCE.get_or_init(|| TypeInfo {
        gc_magic: TYPE_INFO_MAGIC,
        name: StringObject::new(NAME.as_ptr(), NAME.len()),
        num_methods: 0,
        interface_table: ptr::null(),
        is_equal: runtime_error_is_equal,
        hash: runtime_error_hash,
        size: mem::size_of::<RuntimeError>(),
        kind: TypeKind::String as i32,
        pointed_to: TypeId::new_invalid(),
        get_member_offset_runs: None,
    }))
}

#[derive(Debug)]
#[repr(C)]
pub(crate) struct InterfaceTableEntry {
    method_name: StringObject,
    method: FunctionObject,
    method_signature: StringObject,
}

// Only the tests share an interface table across threads (through a OnceLock);
// it is built once and then only read, so that is sound.
#[cfg(test)]
unsafe impl Send for InterfaceTableEntry {}
#[cfg(test)]
unsafe impl Sync for InterfaceTableEntry {}

impl InterfaceTableEntry {
    #[cfg(test)]
    pub(crate) fn new(
        method_name: StringObject,
        method: FunctionObject,
        method_signature: StringObject,
    ) -> Self {
        InterfaceTableEntry {
            method_name,
            method,
            method_signature,
        }
    }

    pub(crate) fn method_name(&self) -> &StringObject {
        &self.method_name
    }

    pub(crate) fn method_signature(&self) -> &StringObject {
        &self.method_signature
    }

    #[cfg(test)]
    pub(crate) fn method(&self) -> &FunctionObject {
        &self.method
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
        let type_id = TypeId::from_ptr(
            &PANIC_NIL_ERROR_TYPE_ID_DUMMY as *const u8 as *const crate::type_id::TypeInfo,
        );
        Self { receiver, type_id }
    }

    pub(crate) fn new_runtime_error(ctx: &mut LightWeightThreadContext, message: &str) -> Self {
        let size = mem::size_of::<RuntimeError>() + message.len() + 1;
        let type_id = runtime_error_type_id();
        let object = ctx.allocate(size, type_id) as *mut u8;
        let data = unsafe { object.add(mem::size_of::<RuntimeError>()) };
        unsafe {
            ptr::copy_nonoverlapping(message.as_ptr(), data, message.len());
            *data.add(message.len()) = 0;
            let message = StringObject::new(data, message.len());
            (object as *mut RuntimeError).write(RuntimeError { message });
        }
        Self::new(ObjectPtr(object as *mut ()), type_id)
    }

    pub(crate) fn is_runtime_error(&self) -> bool {
        self.type_id == runtime_error_type_id()
    }

    pub(crate) fn panic_type_name(type_id: &TypeId) -> String {
        let name = type_id.name().to_str().unwrap_or("");
        match name {
            "BoolObject" => "bool".to_string(),
            "Complex64Object" => "complex64".to_string(),
            "Complex128Object" => "complex128".to_string(),
            "Float32Object" => "float32".to_string(),
            "Float64Object" => "float64".to_string(),
            "IntObject" => "int".to_string(),
            "Int8Object" => "int8".to_string(),
            "Int16Object" => "int16".to_string(),
            "Int32Object" => "int32".to_string(),
            "Int64Object" => "int64".to_string(),
            "StringObject" => "string".to_string(),
            "UintObject" => "uint".to_string(),
            "Uint8Object" => "uint8".to_string(),
            "Uint16Object" => "uint16".to_string(),
            "Uint32Object" => "uint32".to_string(),
            "Uint64Object" => "uint64".to_string(),
            "UintptrObject" => "uintptr".to_string(),
            "UnsafePointerObject" => "unsafe.Pointer".to_string(),
            _ => name.to_string(),
        }
    }

    fn runtime_error_message(&self) -> Option<StringObject> {
        if !self.is_runtime_error() || self.receiver.is_null() {
            return None;
        }
        let error = unsafe { &*(self.receiver.0 as *const RuntimeError) };
        Some(error.message.clone())
    }

    pub fn is_nil(&self) -> bool {
        self.receiver.is_null() && self.type_id == TypeId::new_invalid()
    }

    pub fn is_panic_nil_error(&self) -> bool {
        self.type_id
            == TypeId::from_ptr(
                &PANIC_NIL_ERROR_TYPE_ID_DUMMY as *const u8 as *const crate::type_id::TypeInfo,
            )
    }

    pub fn receiver(&self) -> &ObjectPtr {
        &self.receiver
    }

    pub fn type_id(&self) -> &TypeId {
        &self.type_id
    }

    pub fn search(&self, method_name: StringObject) -> Option<FunctionObject> {
        if self.is_nil() || !self.type_id.is_valid() {
            return None;
        }
        let table = self.type_id.interface_table();
        for entry in table {
            if entry.method_name == method_name {
                return Some(entry.method.clone());
            }
        }
        None
    }

    pub fn search_by_signature(
        &self,
        method_name: &StringObject,
        method_signature: &StringObject,
    ) -> bool {
        let table = self.type_id.interface_table();
        for entry in table {
            if &entry.method_name == method_name && &entry.method_signature == method_signature {
                return true;
            }
        }
        false
    }

    pub fn panic_print(&self) {
        if let Some(message) = self.runtime_error_message() {
            eprintln!("panic: {}", String::from_utf8_lossy(message.as_bytes()));
        } else if self.is_panic_nil_error() {
            eprintln!("panic: panic called with nil argument");
        } else if self.receiver.is_null() {
            eprintln!("panic: nil");
        } else {
            eprintln!("panic: {:?}", self);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_interface_nil() {
        let iface = Interface::nil();
        assert!(iface.is_nil());
        assert!(!iface.is_panic_nil_error());
    }

    #[test]
    fn test_interface_nil_receiver_is_null() {
        let iface = Interface::nil();
        assert!(iface.receiver().is_null());
    }

    #[test]
    fn test_interface_nil_type_id_is_invalid() {
        let iface = Interface::nil();
        assert_eq!(*iface.type_id(), TypeId::new_invalid());
    }

    #[test]
    fn test_interface_panic_nil_error() {
        let iface = Interface::panic_nil_error();
        assert!(!iface.is_nil());
        assert!(iface.is_panic_nil_error());
    }

    #[test]
    fn test_interface_panic_nil_error_receiver_not_null() {
        let iface = Interface::panic_nil_error();
        assert!(!iface.receiver().is_null());
    }

    #[test]
    fn test_interface_new() {
        let dummy: u8 = 0;
        let receiver = ObjectPtr(&dummy as *const u8 as *mut ());
        let type_id =
            TypeId::from_ptr(std::ptr::without_provenance::<crate::type_id::TypeInfo>(42));
        let iface = Interface::new(receiver, type_id);
        assert!(!iface.is_nil());
        assert!(!iface.is_panic_nil_error());
        assert!(!iface.receiver().is_null());
        assert_eq!(
            *iface.type_id(),
            TypeId::from_ptr(std::ptr::without_provenance::<crate::type_id::TypeInfo>(42))
        );
    }

    #[test]
    fn test_interface_panic_print_nil() {
        let iface = Interface::nil();
        iface.panic_print();
    }

    #[test]
    fn test_interface_panic_print_panic_nil_error() {
        let iface = Interface::panic_nil_error();
        iface.panic_print();
    }

    #[test]
    fn test_interface_panic_print_with_receiver() {
        let dummy: u8 = 0;
        let receiver = ObjectPtr(&dummy as *const u8 as *mut ());
        let type_id =
            TypeId::from_ptr(std::ptr::without_provenance::<crate::type_id::TypeInfo>(42));
        let iface = Interface::new(receiver, type_id);
        iface.panic_print();
    }

    #[test]
    fn test_interface_clone() {
        let dummy: u8 = 0;
        let receiver = ObjectPtr(&dummy as *const u8 as *mut ());
        let type_id =
            TypeId::from_ptr(std::ptr::without_provenance::<crate::type_id::TypeInfo>(42));
        let iface = Interface::new(receiver, type_id);
        let cloned = iface.clone();
        assert_eq!(*cloned.type_id(), *iface.type_id());
    }
}
