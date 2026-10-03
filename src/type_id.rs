use std::ffi;
use std::mem;
use std::slice;

use super::ObjectPtr;
use crate::object::interface::InterfaceTableEntry;
use crate::object::string::StringObject;

/// Slot kinds reported by a per-type enumerator function. The meaning of the
/// reported (offset, size, typ) is:
///   GC_SLOT_POINTER      `size` bytes at `offset` are words; each word is a
///                        pointer to an object of type `typ` (its pointee).
///   GC_SLOT_SLICE        a 24-byte slice header at `offset`; its backing array
///                        holds `len` values of the `typ` element type.
///   GC_SLOT_INTERFACE    a 16-byte interface value at `offset`; `typ` unused,
///                        the pointed-to type is read from the value itself.
///   GC_SLOT_STRING       a 16-byte string value at `offset`; `typ` unused, the
///                        raw word is marked so the string buffer stays alive.
///   GC_SLOT_FUNCTION     `size` bytes at `offset` are FunctionObject words; a
///                        tagged word resolves a closure (traced with the
///                        closure's own capture descriptor).
///   GC_SLOT_DEFER_STACK  a word at `offset` that points at the head of a
///                        DeferStackEntry chain (traced by walking `next`).
///   GC_SLOT_RAW          `size` bytes at `offset` that no Go type describes
///                        (a frame's common block, an aggregate without an
///                        enumerator of its own, a map, a channel, ...); the
///                        words are marked one by one, which is sound because
///                        maps and channels are registered with the type of
///                        what they own.
pub(crate) const GC_SLOT_POINTER: i32 = 0;
pub(crate) const GC_SLOT_SLICE: i32 = 1;
pub(crate) const GC_SLOT_INTERFACE: i32 = 2;
pub(crate) const GC_SLOT_STRING: i32 = 3;
pub(crate) const GC_SLOT_FUNCTION: i32 = 4;
pub(crate) const GC_SLOT_DEFER_STACK: i32 = 5;
pub(crate) const GC_SLOT_RAW: i32 = 6;

/// C signature `TypeOffsetVisitor`:
/// `void (*)(uintptr_t offset, uintptr_t size, TypeId type, int kind, void *arg)`.
pub(crate) type TypeOffsetVisitor =
    extern "C" fn(offset: usize, size: usize, typ: TypeId, kind: i32, arg: *mut ffi::c_void);

/// C signature of the per-type enumerator function stored in `TypeInfo`: it
/// reports every pointer-bearing member of an object by calling `visit` with
/// `base`-relative member information, mirroring the generated
/// `get_member_offset_runs_<type>` functions.
pub(crate) type GetMemberOffsetRunsFunc =
    extern "C" fn(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void);

/// The layout class of a value of a type, carried in `TypeInfo`. It tells the
/// collector how to trace a value of this type when it is reached during a
/// root-driven scan, replacing the per-allocation type registration.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub(crate) enum TypeKind {
    None = 0,
    NoPointer = 1,
    String = 2,
    Pointer = 3,
    Map = 4,
    Interface = 5,
    StructArray = 6,
    Slice = 7,
    Function = 8,
}

/// The word every `TypeInfo` starts with; see `TypeId::is_valid`.
pub(crate) const TYPE_INFO_MAGIC: u64 = 0x6767_6f67_6f67_6700;

#[repr(C)]
pub(crate) struct TypeInfo {
    pub(crate) gc_magic: u64,
    pub(crate) name: StringObject,
    pub(crate) num_methods: usize,
    pub(crate) interface_table: *const InterfaceTableEntry,
    pub(crate) is_equal: extern "C" fn(ObjectPtr, ObjectPtr) -> bool,
    pub(crate) hash: extern "C" fn(ObjectPtr) -> usize,
    pub(crate) size: usize,
    /// GcTypeKind at offset 40 (before pointed_to, so the `[repr(C)]` struct
    /// keeps natural alignment for the TypeId that follows).
    pub(crate) kind: i32,
    /// For Pointer/Slice kinds: the type pointed at / held by an element.
    /// Zero (`TypeId(0)`) when there is nothing to point at.
    pub(crate) pointed_to: TypeId,
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

    /// The raw word representation of this TypeId (the `TypeInfo` pointer), the
    /// inverse of `from_raw`.
    #[cfg(test)]
    pub(crate) fn to_raw(self) -> usize {
        self.0
    }

    pub(crate) fn from_raw(val: usize) -> Self {
        TypeId(val)
    }

    /// Whether the word really is a type descriptor. A value that the
    /// collector was told is an interface only carries a type word if it
    /// really is one, so the word is checked against the magic every
    /// `TypeInfo` starts with before anything is dereferenced. The load is
    /// unaligned on purpose: a word that turns out not to be a `TypeId` may
    /// sit at any address, and a misaligned typed dereference is undefined
    /// behaviour (and aborts a debug build).
    pub(crate) fn is_valid(&self) -> bool {
        if self.0 == 0 || !self.0.is_multiple_of(mem::align_of::<u64>()) {
            return false;
        }
        unsafe { std::ptr::read_unaligned(self.0 as *const u64) == TYPE_INFO_MAGIC }
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

    /// The layout class of values of this type; `TypeKind::None` for the
    /// invalid TypeId 0 and for values the collector must not interpret.
    pub(crate) fn kind(&self) -> TypeKind {
        if self.0 == 0 {
            return TypeKind::None;
        }
        match self.type_info().kind {
            v if v == TypeKind::NoPointer as i32 => TypeKind::NoPointer,
            v if v == TypeKind::String as i32 => TypeKind::String,
            v if v == TypeKind::Pointer as i32 => TypeKind::Pointer,
            v if v == TypeKind::Map as i32 => TypeKind::Map,
            v if v == TypeKind::Interface as i32 => TypeKind::Interface,
            v if v == TypeKind::StructArray as i32 => TypeKind::StructArray,
            v if v == TypeKind::Slice as i32 => TypeKind::Slice,
            v if v == TypeKind::Function as i32 => TypeKind::Function,
            _ => TypeKind::None,
        }
    }

    /// The pointee / element type of Pointer and Slice kinds; TypeId 0 when
    /// the type carries no pointee or the TypeId is invalid.
    pub(crate) fn pointed_to(&self) -> TypeId {
        if self.0 == 0 {
            return TypeId(0);
        }
        self.type_info().pointed_to
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

/// A FakeTypeInfo is read-only once it is built, which is what the test tables
/// that hold them in a `OnceLock` rely on.
#[cfg(test)]
unsafe impl Send for FakeTypeInfo {}
#[cfg(test)]
unsafe impl Sync for FakeTypeInfo {}

#[cfg(test)]
impl FakeTypeInfo {
    pub(crate) fn of_kind(kind: TypeKind) -> Self {
        Self::new_with(kind, TypeId::new_invalid(), None, 0)
    }

    /// 8 bytes: a scalar, or anything the collector must never look inside.
    pub(crate) fn no_pointer() -> Self {
        Self::of_kind(TypeKind::NoPointer)
    }

    /// 16 bytes: a data pointer and a length.
    pub(crate) fn string() -> Self {
        Self::new_with(TypeKind::String, TypeId::new_invalid(), None, 16)
    }

    /// 8 bytes: one pointer to a value of `pointed_to`.
    pub(crate) fn pointer(pointed_to: TypeId) -> Self {
        Self::new_with(TypeKind::Pointer, pointed_to, None, 8)
    }

    /// 8 bytes: one pointer to a MapObject.
    pub(crate) fn map() -> Self {
        Self::new_with(TypeKind::Map, TypeId::new_invalid(), None, 8)
    }

    /// 16 bytes: a receiver and a concrete type.
    pub(crate) fn interface_() -> Self {
        Self::new_with(TypeKind::Interface, TypeId::new_invalid(), None, 16)
    }

    /// 8 bytes: a FunctionObject word.
    pub(crate) fn function() -> Self {
        Self::new_with(TypeKind::Function, TypeId::new_invalid(), None, 8)
    }

    /// 24 bytes: a data pointer, a length and a capacity.
    pub(crate) fn slice_of(elem: TypeId) -> Self {
        Self::new_with(TypeKind::Slice, elem, None, 24)
    }

    pub(crate) fn struct_array(get_member_offset_runs: Option<GetMemberOffsetRunsFunc>) -> Self {
        Self::new_with(
            TypeKind::StructArray,
            TypeId::new_invalid(),
            get_member_offset_runs,
            0,
        )
    }

    pub(crate) fn new_with_size(size: usize) -> Self {
        Self::new_with(TypeKind::NoPointer, TypeId::new_invalid(), None, size)
    }

    pub(crate) fn new_with(
        kind: TypeKind,
        pointed_to: TypeId,
        get_member_offset_runs: Option<GetMemberOffsetRunsFunc>,
        size: usize,
    ) -> Self {
        Self::new_with_kind(kind as i32, pointed_to, get_member_offset_runs, size)
    }

    /// Builds a descriptor from a raw discriminant, so a test can look up a
    /// value the enum does not have. Every byte is written through the leaked
    /// pointer, because writing through a copy of the address would be a
    /// stacked borrows violation.
    fn new_with_kind(
        kind: i32,
        pointed_to: TypeId,
        get_member_offset_runs: Option<GetMemberOffsetRunsFunc>,
        size: usize,
    ) -> Self {
        let blob_len = TYPE_INFO_SIZE;
        let blob: Box<[u64]> = vec![0u64; blob_len.div_ceil(8)].into_boxed_slice();
        let leaked: &'static mut [u64] = Box::leak(blob);
        let ptr = leaked.as_mut_ptr() as *mut u8;
        unsafe {
            ptr.cast::<u64>().write(TYPE_INFO_MAGIC);
            ptr.add(mem::offset_of!(TypeInfo, size))
                .cast::<usize>()
                .write(size);
            ptr.add(mem::offset_of!(TypeInfo, kind))
                .cast::<i32>()
                .write(kind);
            ptr.add(mem::offset_of!(TypeInfo, pointed_to))
                .cast::<TypeId>()
                .write(pointed_to);
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
    fn test_type_id_kind() {
        assert_eq!(TypeId::new_invalid().kind(), TypeKind::None);
        assert_eq!(FakeTypeInfo::no_pointer().tid().kind(), TypeKind::NoPointer);
        assert_eq!(FakeTypeInfo::string().tid().kind(), TypeKind::String);
        assert_eq!(FakeTypeInfo::map().tid().kind(), TypeKind::Map);
        assert_eq!(FakeTypeInfo::interface_().tid().kind(), TypeKind::Interface);
        assert_eq!(FakeTypeInfo::function().tid().kind(), TypeKind::Function);
        assert_eq!(
            FakeTypeInfo::slice_of(TypeId::new_invalid()).tid().kind(),
            TypeKind::Slice
        );
        assert_eq!(
            FakeTypeInfo::struct_array(None).tid().kind(),
            TypeKind::StructArray
        );
    }

    #[test]
    fn test_type_id_kind_is_none_for_unknown_discriminant() {
        let blob = FakeTypeInfo::new_with_kind(0x7FFF_FFFF, TypeId::new_invalid(), None, 0);
        let tid = blob.tid();
        assert_eq!(tid.kind(), TypeKind::None);
    }

    #[test]
    fn test_type_id_pointed_to() {
        let pointee = FakeTypeInfo::no_pointer().tid();
        let ptr = FakeTypeInfo::pointer(pointee);
        assert_eq!(ptr.tid().kind(), TypeKind::Pointer);
        assert_eq!(ptr.tid().pointed_to(), pointee);
        assert_eq!(TypeId::new_invalid().pointed_to(), TypeId::new_invalid());
        assert_eq!(
            FakeTypeInfo::map().tid().pointed_to(),
            TypeId::new_invalid()
        );
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
            FakeTypeInfo::no_pointer()
                .tid()
                .get_member_offset_runs()
                .is_none()
        );
        let probe = test_probe_get_member_offset_runs as GetMemberOffsetRunsFunc;
        let fake = FakeTypeInfo::struct_array(Some(probe));
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
            raw.as_mut_ptr()
                .cast::<u8>()
                .add(mem::offset_of!(TypeInfo, name))
                .cast::<StringObject>()
                .write(s);
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
