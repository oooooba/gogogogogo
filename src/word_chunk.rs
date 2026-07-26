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
        unsafe {
            let count = ptr::read(self_ptr as *const usize);
            let buf_ptr = (self_ptr as *const u8).add(mem::size_of::<usize>()) as *const T;
            slice::from_raw_parts(buf_ptr, count)
        }
    }

    /// SAFETY: `self_ptr` must point to a valid WordChunk whose buffer data
    /// (count elements starting at self_ptr + size_of::<usize>()) is accessible.
    pub(crate) unsafe fn duplicate_raw(
        self_ptr: *const Self,
        allocator: &mut dyn ObjectAllocator,
    ) -> ptr::NonNull<Self> {
        let count = unsafe { ptr::read(self_ptr as *const usize) };
        let size = mem::size_of::<WordChunk>() + mem::size_of::<*const ()>() * count;
        let p = allocator.allocate(size, |_| {}) as *mut Self;

        unsafe { (*ptr::addr_of_mut!((*p).count)) = count };
        let src = unsafe {
            let src_ptr = (self_ptr as *const u8).add(mem::size_of::<usize>()) as *const *const ();
            slice::from_raw_parts(src_ptr, count)
        };
        let dst = unsafe {
            slice::from_raw_parts_mut(ptr::addr_of_mut!((*p).buf) as *mut *const (), count)
        };
        dst.copy_from_slice(src);

        ptr::NonNull::new(p).expect("allocator returned null pointer")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;

    struct MockAllocator {
        allocated: Vec<(*mut (), usize)>,
    }

    impl MockAllocator {
        fn new() -> Self {
            MockAllocator {
                allocated: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockAllocator {
        fn allocate(&mut self, size: usize, _destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = size.div_ceil(alignment) * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated.push((ptr, size));
            ptr
        }
        fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
            self.allocate(num_pages * 4096, |_| {})
        }
    }

    impl Drop for MockAllocator {
        fn drop(&mut self) {
            for (ptr, size) in &self.allocated {
                unsafe {
                    let _ = Vec::from_raw_parts(*ptr as *mut isize, 0, *size);
                }
            }
        }
    }

    fn build_word_chunk(count: usize, values: &[usize]) -> Vec<usize> {
        assert_eq!(values.len(), count);
        let mut buf = Vec::with_capacity(1 + count);
        buf.push(count);
        buf.extend_from_slice(values);
        buf
    }

    #[test]
    fn test_as_slice_raw_empty() {
        let buf = build_word_chunk(0, &[]);
        let ptr = buf.as_ptr() as *const WordChunk;
        let slice: &[usize] = unsafe { WordChunk::as_slice_raw(ptr) };
        assert_eq!(slice.len(), 0);
    }

    #[test]
    fn test_as_slice_raw_nonempty() {
        let buf = build_word_chunk(3, &[10, 20, 30]);
        let ptr = buf.as_ptr() as *const WordChunk;
        let slice: &[usize] = unsafe { WordChunk::as_slice_raw(ptr) };
        assert_eq!(slice, &[10, 20, 30]);
    }

    #[test]
    fn test_as_slice_raw_single_element() {
        let buf = build_word_chunk(1, &[42]);
        let ptr = buf.as_ptr() as *const WordChunk;
        let slice: &[usize] = unsafe { WordChunk::as_slice_raw(ptr) };
        assert_eq!(slice, &[42]);
    }

    #[test]
    fn test_duplicate_raw_empty() {
        let mut alloc = MockAllocator::new();
        let buf = build_word_chunk(0, &[]);
        let src = buf.as_ptr() as *const WordChunk;
        let dst = unsafe { WordChunk::duplicate_raw(src, &mut alloc) };
        let slice: &[usize] = unsafe { WordChunk::as_slice_raw(dst.as_ptr()) };
        assert_eq!(slice.len(), 0);
    }

    #[test]
    fn test_duplicate_raw_nonempty() {
        let mut alloc = MockAllocator::new();
        let buf = build_word_chunk(3, &[100, 200, 300]);
        let src = buf.as_ptr() as *const WordChunk;
        let dst = unsafe { WordChunk::duplicate_raw(src, &mut alloc) };
        let slice: &[usize] = unsafe { WordChunk::as_slice_raw(dst.as_ptr()) };
        assert_eq!(slice, &[100, 200, 300]);
    }

    #[test]
    fn test_duplicate_raw_independent_of_source() {
        let mut alloc = MockAllocator::new();
        let buf = build_word_chunk(2, &[7, 8]);
        let src = buf.as_ptr() as *const WordChunk;
        let dst = unsafe { WordChunk::duplicate_raw(src, &mut alloc) };
        let slice_src: &[usize] = unsafe { WordChunk::as_slice_raw(src) };
        let slice_dst: &[usize] = unsafe { WordChunk::as_slice_raw(dst.as_ptr()) };
        assert_eq!(slice_src, slice_dst);
        assert_eq!(slice_dst, &[7, 8]);
    }
}
