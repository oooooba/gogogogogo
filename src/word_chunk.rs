use std::mem;
use std::ptr;
use std::slice;

use crate::ObjectAllocator;

#[repr(C)]
pub struct WordChunk {
    count: usize,
    buf: [*const (); 0],
}

impl WordChunk {
    pub(crate) fn as_slice<T>(&self) -> &[T] {
        unsafe { slice::from_raw_parts(self.buf.as_ptr() as *const T, self.count) }
    }

    pub(crate) fn duplicate(&self, allocator: &mut dyn ObjectAllocator) -> ptr::NonNull<Self> {
        let size = mem::size_of::<WordChunk>() + mem::size_of::<*const ()>() * self.count;
        let p = allocator.allocate(size, |_| {}) as *mut Self;

        unsafe {
            let duplicated = &mut *p;
            duplicated.count = self.count;
            slice::from_raw_parts_mut(duplicated.buf.as_mut_ptr(), duplicated.count)
        }
        .copy_from_slice(self.as_slice());

        unsafe { ptr::NonNull::new_unchecked(p) }
    }
}
