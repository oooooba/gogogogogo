use std::mem;
use std::ptr;
use std::slice;

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

    /// Number of buffer words of the WordChunk at `self_ptr`.
    ///
    /// SAFETY: `self_ptr` must point to a valid WordChunk.
    pub(crate) unsafe fn count_of_raw(self_ptr: *const Self) -> usize {
        unsafe { ptr::read(self_ptr as *const usize) }
    }

    /// Number of bytes of raw storage required to hold a WordChunk whose
    /// buffer holds `count` words.
    pub(crate) fn size_for_count(count: usize) -> usize {
        mem::size_of::<WordChunk>() + mem::size_of::<*const ()>() * count
    }

    /// Copies the WordChunk at `src` into the raw storage starting at `dst` and
    /// returns `dst`.
    ///
    /// SAFETY: `src` must point to a valid WordChunk, and `dst` must be a
    /// non-null pointer to writable storage of at least
    /// `size_for_count(count_of_raw(src))` bytes.
    pub(crate) unsafe fn copy_into_raw(dst: *mut Self, src: *const Self) -> ptr::NonNull<Self> {
        let count = unsafe { Self::count_of_raw(src) };
        unsafe { ptr::write(ptr::addr_of_mut!((*dst).count), count) };
        let src_buf = unsafe {
            let src_ptr = (src as *const u8).add(mem::size_of::<usize>()) as *const *const ();
            slice::from_raw_parts(src_ptr, count)
        };
        let dst_buf = unsafe {
            slice::from_raw_parts_mut(ptr::addr_of_mut!((*dst).buf) as *mut *const (), count)
        };
        dst_buf.copy_from_slice(src_buf);
        ptr::NonNull::new(dst).expect("destination pointer must not be null")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

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
    fn test_size_for_count_matches_raw_layout() {
        assert_eq!(WordChunk::size_for_count(0), mem::size_of::<WordChunk>());
        assert_eq!(
            WordChunk::size_for_count(3),
            mem::size_of::<WordChunk>() + 3 * mem::size_of::<*const ()>()
        );
    }

    #[test]
    fn test_count_of_raw() {
        let buf = build_word_chunk(2, &[11, 22]);
        let count = unsafe { WordChunk::count_of_raw(buf.as_ptr() as *const WordChunk) };
        assert_eq!(count, 2);
    }

    #[test]
    fn test_copy_into_raw_copies_count_and_buffer() {
        let src = build_word_chunk(3, &[1, 2, 3]);
        let mut dst: Vec<usize> = vec![0xFFFF; 1 + 3];
        unsafe {
            WordChunk::copy_into_raw(
                dst.as_mut_ptr() as *mut WordChunk,
                src.as_ptr() as *const WordChunk,
            );
        }
        assert_eq!(dst, vec![3, 1, 2, 3]);
        let copied: &[usize] = unsafe { WordChunk::as_slice_raw(dst.as_ptr() as *const WordChunk) };
        assert_eq!(copied, &[1usize, 2, 3]);
    }

    #[test]
    fn test_copy_into_raw_empty_buffer() {
        let src = build_word_chunk(0, &[]);
        let mut dst: Vec<usize> = vec![0xFFFF];
        unsafe {
            WordChunk::copy_into_raw(
                dst.as_mut_ptr() as *mut WordChunk,
                src.as_ptr() as *const WordChunk,
            );
        }
        assert_eq!(dst, vec![0]);
    }
}
