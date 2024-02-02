use std::ffi;
use std::slice;

use crate::ObjectAllocator;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct StringObject(*const u8);

impl StringObject {
    fn new(p: *const u8) -> Self {
        Self(p)
    }

    pub(crate) fn builder(
        len_in_bytes: usize,
        allocator: &mut dyn ObjectAllocator,
    ) -> StringObjectBuilder {
        StringObjectBuilder::new(len_in_bytes, allocator)
    }

    pub(crate) fn len_in_bytes(&self) -> usize {
        unsafe { libc::strlen(self.0 as *const libc::c_char) }
    }

    pub(crate) fn as_bytes(&self) -> &[u8] {
        unsafe { ffi::CStr::from_ptr(self.0 as *const libc::c_char).to_bytes() }
    }
}

pub(crate) struct StringObjectBuilder {
    ptr: *mut u8,
    len_in_bytes: usize,
    cursor: usize,
}

impl StringObjectBuilder {
    fn new(len_in_bytes: usize, allocator: &mut dyn ObjectAllocator) -> Self {
        let ptr = allocator.allocate(len_in_bytes + 1, |_| {}) as *mut u8;
        Self {
            ptr,
            len_in_bytes,
            cursor: 0,
        }
    }

    fn as_mut_slice(&self) -> &mut [u8] {
        unsafe { slice::from_raw_parts_mut(self.ptr, self.len_in_bytes + 1) }
    }

    pub fn append_bytes(&mut self, src: &[u8]) {
        assert!(self.cursor <= self.len_in_bytes);

        let bytes = self.as_mut_slice();
        let len = src.len();
        bytes[self.cursor..(self.cursor + len)].clone_from_slice(&src[..len]);
        self.cursor += len;

        assert!(self.cursor <= self.len_in_bytes);
    }

    pub fn append_char(&mut self, src: char) {
        assert!(self.cursor < self.len_in_bytes);

        let bytes = self.as_mut_slice();
        let len = src.encode_utf8(&mut bytes[self.cursor..]).len();
        self.cursor += len;

        assert!(self.cursor <= self.len_in_bytes);
    }

    pub fn build(self) -> StringObject {
        assert!(self.cursor == self.len_in_bytes);

        let bytes = self.as_mut_slice();
        bytes[self.cursor] = 0;
        StringObject::new(self.ptr)
    }
}
