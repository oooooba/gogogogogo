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

    pub(crate) fn duplicate(&self) -> Self {
        Self::new(self.ptr, self.size, self.capacity)
    }

    pub(crate) fn size(&self) -> usize {
        self.size
    }

    pub(crate) fn capacity(&self) -> usize {
        self.capacity
    }

    pub(crate) fn as_raw_slice<T>(&self) -> &[T] {
        unsafe { slice::from_raw_parts(self.ptr as *const T, self.capacity) }
    }

    pub(crate) fn as_raw_slice_mut<T>(&mut self) -> &mut [T] {
        unsafe { slice::from_raw_parts_mut(self.ptr as *mut T, self.capacity) }
    }
}
