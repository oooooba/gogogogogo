use std::slice;

#[cfg(test)]
use crate::ObjectAllocatorPtr;
#[cfg(test)]
use crate::type_id::TypeId;

#[derive(Clone, Eq, Debug)]
#[repr(C)]
pub struct StringObject(*const u8, usize);

impl PartialEq for StringObject {
    fn eq(&self, other: &Self) -> bool {
        self.as_bytes() == other.as_bytes()
    }
}

impl StringObject {
    pub(crate) fn new(p: *const u8, len_in_bytes: usize) -> Self {
        Self(p, len_in_bytes)
    }

    #[cfg(test)]
    pub(crate) fn builder(
        len_in_bytes: usize,
        allocator: &ObjectAllocatorPtr,
    ) -> StringObjectBuilder {
        StringObjectBuilder::new(len_in_bytes, allocator)
    }

    pub(crate) fn builder_with_buffer(len_in_bytes: usize, ptr: *mut u8) -> StringObjectBuilder {
        StringObjectBuilder::new_with_buffer(len_in_bytes, ptr)
    }

    pub(crate) fn len_in_bytes(&self) -> usize {
        self.1
    }

    pub(crate) fn as_bytes(&self) -> &[u8] {
        if self.0.is_null() {
            &[]
        } else {
            unsafe { slice::from_raw_parts(self.0, self.1) }
        }
    }

    pub(crate) fn to_str(&self) -> Result<&str, std::str::Utf8Error> {
        std::str::from_utf8(self.as_bytes())
    }
}

pub(crate) struct StringObjectBuilder {
    ptr: *mut u8,
    len_in_bytes: usize,
    cursor: usize,
}

impl StringObjectBuilder {
    #[cfg(test)]
    fn new(len_in_bytes: usize, allocator: &ObjectAllocatorPtr) -> Self {
        let ptr = allocator.allocate(len_in_bytes + 1, TypeId::new_invalid()) as *mut u8;
        Self::new_with_buffer(len_in_bytes, ptr)
    }

    fn new_with_buffer(len_in_bytes: usize, ptr: *mut u8) -> Self {
        assert!(!ptr.is_null());
        Self {
            ptr,
            len_in_bytes,
            cursor: 0,
        }
    }

    fn as_mut_slice(&mut self) -> &mut [u8] {
        unsafe { slice::from_raw_parts_mut(self.ptr, self.len_in_bytes + 1) }
    }

    pub fn append_bytes(&mut self, src: &[u8]) {
        assert!(self.cursor <= self.len_in_bytes);

        let index = self.cursor;
        let bytes = self.as_mut_slice();
        let len = src.len();
        bytes[index..(index + len)].clone_from_slice(&src[..len]);
        self.cursor += len;

        assert!(self.cursor <= self.len_in_bytes);
    }

    pub fn append_char(&mut self, src: char) {
        assert!(self.cursor < self.len_in_bytes);

        let index = self.cursor;
        let bytes = self.as_mut_slice();
        let len = src.encode_utf8(&mut bytes[index..]).len();
        self.cursor += len;

        assert!(self.cursor <= self.len_in_bytes);
    }

    pub fn build(mut self) -> StringObject {
        assert!(self.cursor == self.len_in_bytes);

        let index = self.cursor;
        let bytes = self.as_mut_slice();
        bytes[index] = 0;
        StringObject::new(self.ptr, self.len_in_bytes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;

    #[test]
    fn test_string_object_builder_and_as_bytes() {
        let allocator = ObjectAllocator::new();
        let mut builder = StringObject::builder(5, &allocator.ptr());
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.as_bytes(), b"hello");
    }

    #[test]
    fn test_string_object_len_in_bytes() {
        let allocator = ObjectAllocator::new();
        let mut builder = StringObject::builder(5, &allocator.ptr());
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.len_in_bytes(), 5);
    }

    #[test]
    fn test_string_object_to_str() {
        let allocator = ObjectAllocator::new();
        let mut builder = StringObject::builder(5, &allocator.ptr());
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.to_str().unwrap(), "hello");
    }

    #[test]
    fn test_string_object_eq() {
        let allocator = ObjectAllocator::new();
        let mut builder1 = StringObject::builder(5, &allocator.ptr());
        builder1.append_bytes(b"hello");
        let s1 = builder1.build();

        let mut builder2 = StringObject::builder(5, &allocator.ptr());
        builder2.append_bytes(b"hello");
        let s2 = builder2.build();

        assert_eq!(s1, s2);
    }

    #[test]
    fn test_string_object_ne() {
        let allocator = ObjectAllocator::new();
        let mut builder1 = StringObject::builder(5, &allocator.ptr());
        builder1.append_bytes(b"hello");
        let s1 = builder1.build();

        let mut builder2 = StringObject::builder(5, &allocator.ptr());
        builder2.append_bytes(b"world");
        let s2 = builder2.build();

        assert_ne!(s1, s2);
    }

    #[test]
    fn test_string_object_append_char() {
        let allocator = ObjectAllocator::new();
        let mut builder = StringObject::builder(5, &allocator.ptr());
        builder.append_char('h');
        builder.append_char('e');
        builder.append_char('l');
        builder.append_char('l');
        builder.append_char('o');
        let s = builder.build();
        assert_eq!(s.as_bytes(), b"hello");
    }

    #[test]
    fn test_string_object_empty() {
        let allocator = ObjectAllocator::new();
        let builder = StringObject::builder(0, &allocator.ptr());
        let s = builder.build();
        assert_eq!(s.as_bytes(), b"");
        assert_eq!(s.len_in_bytes(), 0);
        assert_eq!(s.to_str().unwrap(), "");
    }

    #[test]
    fn test_string_object_contains_nul_bytes() {
        let allocator = ObjectAllocator::new();
        let mut builder = StringObject::builder(3, &allocator.ptr());
        builder.append_bytes(b"a\x00b");
        let s = builder.build();

        let mut builder2 = StringObject::builder(3, &allocator.ptr());
        builder2.append_bytes(b"a\x00b");
        let s2 = builder2.build();

        let mut builder3 = StringObject::builder(4, &allocator.ptr());
        builder3.append_bytes(b"a\x00bc");
        let s3 = builder3.build();

        assert_eq!(s.len_in_bytes(), 3);
        assert_eq!(s.as_bytes(), b"a\x00b");
        assert_eq!(s, s2);
        assert_ne!(s, s3);
        assert_eq!(s.to_str().unwrap(), "a\u{0}b");
    }
}
