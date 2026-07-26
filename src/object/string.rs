use std::ffi;
use std::slice;

use crate::ObjectAllocator;

#[derive(Clone, Eq, Debug)]
#[repr(C)]
pub struct StringObject(*const u8);

impl PartialEq for StringObject {
    #[allow(clippy::unconditional_recursion)]
    fn eq(&self, other: &Self) -> bool {
        let lhs = unsafe { ffi::CStr::from_ptr(self.0 as *const libc::c_char) };
        let rhs = unsafe { ffi::CStr::from_ptr(other.0 as *const libc::c_char) };
        lhs == rhs
    }
}

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
        self.as_bytes().len()
    }

    pub(crate) fn as_bytes(&self) -> &[u8] {
        unsafe { ffi::CStr::from_ptr(self.0 as *const libc::c_char).to_bytes() }
    }

    pub(crate) fn to_str(&self) -> Result<&str, std::str::Utf8Error> {
        unsafe { ffi::CStr::from_ptr(self.0 as *const libc::c_char).to_str() }
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
        StringObject::new(self.ptr)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::mem;

    struct AllocatedObject {
        ptr: *mut (),
        size: usize,
        destructor: fn(*mut ()),
    }

    struct MockObjectAllocator {
        allocated_objects: Vec<AllocatedObject>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated_objects: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = size.div_ceil(alignment) * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, _num_pages: usize) -> *mut () {
            unimplemented!()
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for allocated_object in &self.allocated_objects {
                (allocated_object.destructor)(allocated_object.ptr);
                unsafe {
                    Vec::from_raw_parts(
                        allocated_object.ptr as *mut isize,
                        0,
                        allocated_object.size,
                    );
                }
            }
        }
    }

    #[test]
    fn test_string_object_builder_and_as_bytes() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.as_bytes(), b"hello");
    }

    #[test]
    fn test_string_object_len_in_bytes() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.len_in_bytes(), 5);
    }

    #[test]
    fn test_string_object_to_str() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
        builder.append_bytes(b"hello");
        let s = builder.build();
        assert_eq!(s.to_str().unwrap(), "hello");
    }

    #[test]
    fn test_string_object_eq() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder1 = StringObject::builder(5, &mut allocator);
        builder1.append_bytes(b"hello");
        let s1 = builder1.build();

        let mut builder2 = StringObject::builder(5, &mut allocator);
        builder2.append_bytes(b"hello");
        let s2 = builder2.build();

        assert_eq!(s1, s2);
    }

    #[test]
    fn test_string_object_ne() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder1 = StringObject::builder(5, &mut allocator);
        builder1.append_bytes(b"hello");
        let s1 = builder1.build();

        let mut builder2 = StringObject::builder(5, &mut allocator);
        builder2.append_bytes(b"world");
        let s2 = builder2.build();

        assert_ne!(s1, s2);
    }

    #[test]
    fn test_string_object_append_char() {
        let mut allocator = MockObjectAllocator::new();
        let mut builder = StringObject::builder(5, &mut allocator);
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
        let mut allocator = MockObjectAllocator::new();
        let builder = StringObject::builder(0, &mut allocator);
        let s = builder.build();
        assert_eq!(s.as_bytes(), b"");
        assert_eq!(s.len_in_bytes(), 0);
        assert_eq!(s.to_str().unwrap(), "");
    }
}
