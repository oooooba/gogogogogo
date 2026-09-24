use std::ffi;
use std::slice;

#[cfg(test)]
use std::mem;

use super::ObjectPtr;
use crate::object::interface::InterfaceTableEntry;
use crate::object::string::StringObject;

/// C signature `TypeOffsetVisitor`: `void (*)(uintptr_t offset, uintptr_t size, void *arg)`.
pub(crate) type TypeOffsetVisitor =
    extern "C" fn(offset: usize, size: usize, arg: *mut ffi::c_void);

/// C signature of the per-type enumerator function stored in `TypeInfo`: it
/// reports every pointer-bearing member range of an object by calling `visit`
/// with `base`-relative (offset, size) ranges, mirroring the generated
/// `get_member_offset_runs_<type>` functions.
#[allow(dead_code)]
pub(crate) type GetMemberOffsetRunsFunc =
    extern "C" fn(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void);

#[repr(C)]
pub(crate) struct TypeInfo {
    pub(crate) name: StringObject,
    pub(crate) num_methods: usize,
    pub(crate) interface_table: *const InterfaceTableEntry,
    pub(crate) is_equal: extern "C" fn(ObjectPtr, ObjectPtr) -> bool,
    pub(crate) hash: extern "C" fn(ObjectPtr) -> usize,
    pub(crate) size: usize,
    pub(crate) no_pointers: bool,
    pub(crate) is_interface: bool,
    pub(crate) get_member_offset_runs: Option<GetMemberOffsetRunsFunc>,
}

unsafe impl Send for TypeInfo {}
unsafe impl Sync for TypeInfo {}

#[allow(dead_code)]
pub(crate) const TYPE_INFO_SIZE: usize = std::mem::size_of::<TypeInfo>();

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(C)]
pub(crate) struct TypeId(usize);

impl TypeId {
    pub(crate) fn new_invalid() -> Self {
        TypeId(0)
    }

    pub(crate) fn from_raw(val: usize) -> Self {
        TypeId(val)
    }

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

    pub(crate) fn is_no_pointer(&self) -> bool {
        if self.0 == 0 {
            return false;
        }
        self.type_info().no_pointers
    }

    pub(crate) fn is_interface_type(&self) -> bool {
        if self.0 == 0 {
            return false;
        }
        self.type_info().is_interface
    }

    pub(crate) fn get_member_offset_runs(&self) -> Option<GetMemberOffsetRunsFunc> {
        if self.0 == 0 {
            return None;
        }
        self.type_info().get_member_offset_runs
    }

    pub fn is_equal_func(&self) -> extern "C" fn(ObjectPtr, ObjectPtr) -> bool {
        let type_info = self.type_info();
        type_info.is_equal
    }

    pub fn hash_func(&self) -> extern "C" fn(ObjectPtr) -> usize {
        let type_info = self.type_info();
        type_info.hash
    }

    pub(crate) fn name(&self) -> &StringObject {
        &self.type_info().name
    }
}

#[cfg(test)]
pub(crate) struct FakeTypeInfo {
    tid: TypeId,
    holder: *mut [u64],
}

#[cfg(test)]
impl FakeTypeInfo {
    pub(crate) fn new(no_pointers: bool) -> Self {
        Self::new_with_get_member_offset_runs(no_pointers, None)
    }

    pub(crate) fn new_interface(no_pointers: bool) -> Self {
        Self::new_with_flags(no_pointers, true, None, 0)
    }

    pub(crate) fn new_with_size(no_pointers: bool, size: usize) -> Self {
        Self::new_with_flags(no_pointers, false, None, size)
    }

    pub(crate) fn new_with_get_member_offset_runs(
        no_pointers: bool,
        get_member_offset_runs: Option<GetMemberOffsetRunsFunc>,
    ) -> Self {
        Self::new_with_flags(no_pointers, false, get_member_offset_runs, 0)
    }

    // Writes a no_pointers flag, an interface flag, an optional
    // get_member_offset_runs function pointer, and a size into a zeroed
    // TypeInfo-sized blob. The blob is Box::leak'ed and reclaimed via a raw
    // pointer in Drop (Miri Stacked Borrows); writes happen through the leaked
    // marker so the raw pointer tags are not invalidated by its Unique retag.
    pub(crate) fn new_with_flags(
        no_pointers: bool,
        is_interface: bool,
        get_member_offset_runs: Option<GetMemberOffsetRunsFunc>,
        size: usize,
    ) -> Self {
        let blob_len = TYPE_INFO_SIZE;
        let blob: Box<[u64]> = vec![0u64; blob_len.div_ceil(8)].into_boxed_slice();
        let leaked: &'static mut [u64] = Box::leak(blob);
        let ptr = leaked.as_mut_ptr() as *mut u8;
        unsafe {
            ptr.add(mem::offset_of!(TypeInfo, no_pointers))
                .cast::<bool>()
                .write(no_pointers);
            ptr.add(mem::offset_of!(TypeInfo, is_interface))
                .cast::<bool>()
                .write(is_interface);
            ptr.add(mem::offset_of!(TypeInfo, size))
                .cast::<usize>()
                .write(size);
            ptr.add(mem::offset_of!(TypeInfo, get_member_offset_runs))
                .cast::<Option<GetMemberOffsetRunsFunc>>()
                .write(get_member_offset_runs);
        }
        let holder = leaked as *mut [u64];
        FakeTypeInfo {
            tid: TypeId::from_raw(leaked.as_ptr() as usize),
            holder,
        }
    }

    pub(crate) fn tid(&self) -> TypeId {
        self.tid
    }
}

#[cfg(test)]
impl Drop for FakeTypeInfo {
    fn drop(&mut self) {
        unsafe {
            drop(Box::from_raw(self.holder));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_type_id_new_invalid() {
        let id = TypeId::new_invalid();
        assert_eq!(id.0, 0);
    }

    #[test]
    fn test_type_id_from_raw_roundtrip() {
        let id = TypeId::from_raw(42);
        assert_eq!(id.0, 42);
    }

    #[test]
    fn test_type_id_clone_copy() {
        let id = TypeId::from_raw(99);
        let id2 = id;
        assert_eq!(id, id2);
    }

    #[test]
    fn test_type_id_partial_eq() {
        let a = TypeId::from_raw(10);
        let b = TypeId::from_raw(10);
        let c = TypeId::from_raw(20);
        assert_eq!(a, b);
        assert_ne!(a, c);
    }

    #[test]
    fn test_type_id_debug() {
        let id = TypeId::from_raw(7);
        let debug_str = format!("{:?}", id);
        assert!(debug_str.contains("7"));
    }

    #[test]
    fn test_type_id_is_no_pointer() {
        assert!(FakeTypeInfo::new(true).tid().is_no_pointer());
        assert!(!FakeTypeInfo::new(false).tid().is_no_pointer());
        assert!(!TypeId::new_invalid().is_no_pointer());
    }

    #[test]
    fn test_type_id_is_interface_type() {
        assert!(!TypeId::new_invalid().is_interface_type());
        assert!(FakeTypeInfo::new_interface(false).tid().is_interface_type());
        assert!(FakeTypeInfo::new_interface(true).tid().is_interface_type());
        assert!(!FakeTypeInfo::new(false).tid().is_interface_type());
        assert!(!FakeTypeInfo::new(true).tid().is_interface_type());
    }

    extern "C" fn test_probe_get_member_offset_runs(
        _visit: TypeOffsetVisitor,
        _base: usize,
        _arg: *mut std::ffi::c_void,
    ) {
    }

    #[test]
    fn test_type_id_get_member_offset_runs() {
        assert!(TypeId::new_invalid().get_member_offset_runs().is_none());
        assert!(
            FakeTypeInfo::new(false)
                .tid()
                .get_member_offset_runs()
                .is_none()
        );
        let probe = test_probe_get_member_offset_runs as GetMemberOffsetRunsFunc;
        let fake = FakeTypeInfo::new_with_get_member_offset_runs(false, Some(probe));
        assert_eq!(
            fake.tid().get_member_offset_runs().map(|f| f as usize),
            Some(probe as usize)
        );
    }

    #[test]
    fn test_type_id_name() {
        // Build a real (TypeInfo-sized) region containing a name, then point a
        // TypeId at it and read the name back. Miri requires the allocation to
        // cover the whole TypeInfo. Box::leak exposes provenance so the raw
        // type_id integer can re-derive a &TypeInfo; reclaim via a raw pointer.
        let mut raw: Box<[u64]> = vec![0u64; TYPE_INFO_SIZE.div_ceil(8)].into_boxed_slice();
        let s = StringObject::new(b"IntObject".as_ptr(), 9);
        unsafe {
            (raw.as_mut_ptr() as *mut StringObject).write(s);
        }
        let leaked: &'static mut [u64] = Box::leak(raw);
        let blob = leaked as *mut [u64];
        let tid = TypeId::from_raw(leaked.as_ptr() as usize);
        assert_eq!(tid.name().to_str().unwrap(), "IntObject");
        unsafe {
            drop(Box::from_raw(blob));
        }
    }
}
