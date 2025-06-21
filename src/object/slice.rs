use std::slice;

#[repr(C)]
pub struct SliceObject {
    ptr: *mut (),
    size: usize,
    capacity: usize,
}

impl SliceObject {
    pub(crate) fn new(ptr: *mut (), size: usize, capacity: usize) -> Self {
        SliceObject {
            ptr,
            size,
            capacity,
        }
    }

    pub(crate) fn duplicate_extend(&self, addition: usize) -> Self {
        assert!(self.size + addition <= self.capacity);
        Self::new(self.ptr, self.size + addition, self.capacity)
    }

    pub(crate) fn size(&self) -> usize {
        self.size
    }

    pub(crate) fn capacity(&self) -> usize {
        self.capacity
    }

    pub(crate) fn as_bytes(&self, elem_size_in_bytes: usize) -> &[u8] {
        if self.ptr.is_null() {
            assert_eq!(self.capacity, 0);
            assert_eq!(self.size, 0);
            &[]
        } else {
            unsafe {
                slice::from_raw_parts(self.ptr as *const u8, self.capacity * elem_size_in_bytes)
            }
        }
    }

    pub(crate) fn as_bytes_mut(&mut self, elem_size_in_bytes: usize) -> &mut [u8] {
        if self.ptr.is_null() {
            assert_eq!(self.capacity, 0);
            assert_eq!(self.size, 0);
            &mut []
        } else {
            unsafe {
                slice::from_raw_parts_mut(self.ptr as *mut u8, self.capacity * elem_size_in_bytes)
            }
        }
    }
}
