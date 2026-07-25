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

#[cfg(test)]
mod tests {
    use super::*;
    use std::ptr;

    #[test]
    fn test_slice_new() {
        let mut buf = [0u8; 64];
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 5, 10);
        assert_eq!(slice.size(), 5);
        assert_eq!(slice.capacity(), 10);
    }

    #[test]
    fn test_slice_duplicate_extend() {
        let mut buf = [0u8; 64];
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 5, 10);
        let extended = slice.duplicate_extend(3);
        assert_eq!(extended.size(), 8);
        assert_eq!(extended.capacity(), 10);
    }

    #[test]
    fn test_slice_duplicate_extend_to_full() {
        let mut buf = [0u8; 64];
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 5, 10);
        let extended = slice.duplicate_extend(5);
        assert_eq!(extended.size(), 10);
        assert_eq!(extended.capacity(), 10);
    }

    #[test]
    #[should_panic]
    fn test_slice_duplicate_extend_overflow() {
        let mut buf = [0u8; 64];
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 5, 10);
        slice.duplicate_extend(6);
    }

    #[test]
    fn test_slice_as_bytes() {
        let mut buf = [0u8; 64];
        buf[0] = 1;
        buf[1] = 2;
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 2, 10);
        let bytes = slice.as_bytes(1);
        assert_eq!(bytes.len(), 10);
        assert_eq!(bytes[0], 1);
        assert_eq!(bytes[1], 2);
    }

    #[test]
    fn test_slice_as_bytes_null() {
        let slice = SliceObject::new(ptr::null_mut(), 0, 0);
        let bytes = slice.as_bytes(1);
        assert_eq!(bytes, &[]);
    }

    #[test]
    fn test_slice_as_bytes_mut() {
        let mut buf = [0u8; 64];
        let ptr = buf.as_mut_ptr() as *mut ();
        let mut slice = SliceObject::new(ptr, 2, 10);
        {
            let bytes = slice.as_bytes_mut(1);
            bytes[0] = 42;
            bytes[1] = 43;
        }
        assert_eq!(buf[0], 42);
        assert_eq!(buf[1], 43);
    }

    #[test]
    fn test_slice_as_bytes_mut_null() {
        let mut slice = SliceObject::new(ptr::null_mut(), 0, 0);
        let bytes = slice.as_bytes_mut(1);
        assert_eq!(bytes, &mut []);
    }

    #[test]
    fn test_slice_as_bytes_elem_size() {
        let mut buf = [0u32; 16];
        buf[0] = 0x04030201;
        buf[1] = 0x08070605;
        let ptr = buf.as_mut_ptr() as *mut ();
        let slice = SliceObject::new(ptr, 2, 16);
        let bytes = slice.as_bytes(4);
        assert_eq!(bytes.len(), 16 * 4);
        assert_eq!(bytes[0], 1);
        assert_eq!(bytes[3], 4);
        assert_eq!(bytes[4], 5);
    }
}
