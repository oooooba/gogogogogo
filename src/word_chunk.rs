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
    /// SAFETY: `self_ptr` must point to a valid WordChunk whose buffer data
    /// (count elements starting at self_ptr + size_of::<usize>()) is accessible
    /// for the lifetime of the returned slice.
    pub(crate) unsafe fn as_slice_raw<'a, T>(self_ptr: *const Self) -> &'a [T] {
        let count = ptr::read(self_ptr as *const usize);
        let buf_ptr = (self_ptr as *const u8).add(mem::size_of::<usize>()) as *const T;
        slice::from_raw_parts(buf_ptr, count)
    }

    /// SAFETY: `self_ptr` must point to a valid WordChunk whose buffer data
    /// (count elements starting at self_ptr + size_of::<usize>()) is accessible.
    pub(crate) unsafe fn duplicate_raw(
        self_ptr: *const Self,
        allocator: &mut dyn ObjectAllocator,
    ) -> ptr::NonNull<Self> {
        let count = ptr::read(self_ptr as *const usize);
        let size = mem::size_of::<WordChunk>() + mem::size_of::<*const ()>() * count;
        let p = allocator.allocate(size, |_| {}) as *mut Self;

        (*ptr::addr_of_mut!((*p).count)) = count;
        let src = (self_ptr as *const u8).add(mem::size_of::<usize>()) as *const *const ();
        let dst = slice::from_raw_parts_mut(ptr::addr_of_mut!((*p).buf) as *mut *const (), count);
        dst.copy_from_slice(slice::from_raw_parts(src, count));

        ptr::NonNull::new_unchecked(p)
    }
}
