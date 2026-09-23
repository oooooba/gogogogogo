use std::slice;

#[cfg(test)]
use std::mem;

use super::ObjectPtr;
use crate::object::interface::InterfaceTableEntry;
use crate::object::string::StringObject;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(C)]
pub(crate) struct TypeOffsetRun {
    pub(crate) offset: usize,
    pub(crate) size: usize,
}

#[repr(C)]
struct TypeInfo {
    name: StringObject,
    num_methods: usize,
    interface_table: *const InterfaceTableEntry,
    is_equal: extern "C" fn(ObjectPtr, ObjectPtr) -> bool,
    hash: extern "C" fn(ObjectPtr) -> usize,
    size: usize,
    no_pointers: bool,
    num_member_offset_runs: usize,
    member_offset_runs: *const TypeOffsetRun,
}

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

    pub(crate) fn member_offset_runs(&self) -> Option<&[TypeOffsetRun]> {
        if self.0 == 0 {
            return None;
        }
        let type_info = self.type_info();
        if type_info.num_member_offset_runs == 0 {
            return None;
        }
        unsafe {
            Some(slice::from_raw_parts(
                type_info.member_offset_runs,
                type_info.num_member_offset_runs,
            ))
        }
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
        Self::new_with_member_offset_runs(no_pointers, &[])
    }

    // Stores the TypeInfo followed by a copy of `runs` in one allocation, so a
    // single held pointer keeps both alive and freeing the holder releases both
    // without dropping any live shared reference (Miri Stacked Borrows).
    pub(crate) fn new_with_member_offset_runs(no_pointers: bool, runs: &[TypeOffsetRun]) -> Self {
        let blob_len = TYPE_INFO_SIZE + mem::size_of_val(runs);
        let blob: Box<[u64]> = vec![0u64; blob_len.div_ceil(8)].into_boxed_slice();
        let leaked: &'static mut [u64] = Box::leak(blob);
        let ptr = leaked.as_mut_ptr() as *mut u8;
        unsafe {
            ptr.add(mem::offset_of!(TypeInfo, no_pointers))
                .cast::<bool>()
                .write(no_pointers);
            if !runs.is_empty() {
                let runs_ptr = ptr.add(TYPE_INFO_SIZE) as *mut TypeOffsetRun;
                std::ptr::copy_nonoverlapping(runs.as_ptr(), runs_ptr, runs.len());
                ptr.add(mem::offset_of!(TypeInfo, num_member_offset_runs))
                    .cast::<usize>()
                    .write(runs.len());
                ptr.add(mem::offset_of!(TypeInfo, member_offset_runs))
                    .cast::<*const TypeOffsetRun>()
                    .write(runs_ptr);
            }
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
    fn test_type_id_member_offset_runs() {
        assert_eq!(TypeId::new_invalid().member_offset_runs(), None);
        assert_eq!(FakeTypeInfo::new(false).tid().member_offset_runs(), None);
        let runs = [
            TypeOffsetRun {
                offset: 16,
                size: 8,
            },
            TypeOffsetRun {
                offset: 32,
                size: 64,
            },
        ];
        let fake = FakeTypeInfo::new_with_member_offset_runs(false, &runs);
        assert_eq!(
            fake.tid().member_offset_runs(),
            Some(
                &[
                    TypeOffsetRun {
                        offset: 16,
                        size: 8
                    },
                    TypeOffsetRun {
                        offset: 32,
                        size: 64
                    }
                ][..]
            )
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
