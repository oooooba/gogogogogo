use std::collections::BTreeMap;
use std::ffi;
use std::mem;
use std::ptr;
use std::ptr::NonNull;

use allocator_api2::alloc::{AllocError, Allocator, Layout};

pub(crate) struct ObjectAllocator(ObjectAllocatorPtr);

impl ObjectAllocator {
    pub(crate) fn new() -> Self {
        ObjectAllocator(ObjectAllocatorPtr(Box::into_raw(Box::new(
            ObjectAllocatorInner::new(),
        ))))
    }

    pub(crate) fn ptr(&mut self) -> ObjectAllocatorPtr {
        self.0.clone()
    }
}

impl Drop for ObjectAllocator {
    fn drop(&mut self) {
        unsafe {
            Box::from_raw(self.0.0).free_all_allocated_objects();
        }
    }
}

struct ObjectAllocatorInner {
    allocated_objects: BTreeMap<usize, AllocatedObject>,
}

struct AllocatedObject {
    ptr: *mut (),
    size: usize,
    kind: AllocationKind,
}

#[derive(Clone, Copy)]
enum AllocationKind {
    Heap,
    GuardedPages,
}

const ALLOCATION_ALIGNMENT: usize = mem::size_of::<u128>();

impl ObjectAllocatorInner {
    fn new() -> Self {
        ObjectAllocatorInner {
            allocated_objects: BTreeMap::new(),
        }
    }

    fn allocate(&mut self, size: usize) -> *mut () {
        let size = size.div_ceil(ALLOCATION_ALIGNMENT) * ALLOCATION_ALIGNMENT;
        let buf: Vec<u128> = vec![0; size / ALLOCATION_ALIGNMENT];
        let ptr = buf.leak().as_mut_ptr();
        let ptr = ptr as *mut ();
        self.allocated_objects.insert(
            ptr as usize,
            AllocatedObject {
                ptr,
                size,
                kind: AllocationKind::Heap,
            },
        );
        ptr
    }

    fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
        unsafe {
            #[cfg(miri)]
            let protection = libc::PROT_READ | libc::PROT_WRITE;
            #[cfg(not(miri))]
            let protection = libc::PROT_NONE;

            let stack_area_addr = libc::mmap(
                ptr::null_mut(),
                4096 * (num_pages + 1),
                protection,
                libc::MAP_ANONYMOUS | libc::MAP_PRIVATE,
                -1,
                0,
            );
            if stack_area_addr == libc::MAP_FAILED {
                let message = ffi::CString::new("allocate stack area").unwrap();
                libc::perror(message.as_ptr());
                panic!();
            }
            let stack_start_addr = ((stack_area_addr as usize) + 4096) as *mut libc::c_void;
            #[cfg(not(miri))]
            {
                let ret = libc::mprotect(
                    stack_start_addr,
                    4096 * num_pages,
                    libc::PROT_READ | libc::PROT_WRITE,
                );
                if ret != 0 {
                    let message = ffi::CString::new("stack protection mode").unwrap();
                    libc::perror(message.as_ptr());
                    panic!();
                }
            }
            let ptr = stack_start_addr as *mut ();
            self.allocated_objects.insert(
                ptr as usize,
                AllocatedObject {
                    ptr,
                    size: 4096 * num_pages,
                    kind: AllocationKind::GuardedPages,
                },
            );
            ptr
        }
    }

    fn free_all_allocated_objects(&mut self) {
        for object in self.allocated_objects.values() {
            match object.kind {
                AllocationKind::Heap => unsafe {
                    Vec::from_raw_parts(
                        object.ptr as *mut u128,
                        0,
                        object.size / ALLOCATION_ALIGNMENT,
                    );
                },
                AllocationKind::GuardedPages => {
                    let base_addr = (object.ptr as usize - 4096) as *mut libc::c_void;
                    unsafe {
                        libc::munmap(base_addr, object.size + 4096);
                    }
                }
            }
        }
    }
}

#[derive(Clone)]
pub(crate) struct ObjectAllocatorPtr(*mut ObjectAllocatorInner);

impl ObjectAllocatorPtr {
    pub(crate) fn allocate(&self, size: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate(size)
    }

    pub(crate) fn allocate_guarded_pages(&self, num_pages: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate_guarded_pages(num_pages)
    }
}

unsafe impl Allocator for ObjectAllocatorPtr {
    fn allocate(&self, layout: Layout) -> Result<NonNull<[u8]>, AllocError> {
        let ptr = if layout.align() <= ALLOCATION_ALIGNMENT {
            self.allocate(layout.size())
        } else {
            let total = layout.size() + layout.align();
            let base = self.allocate(total) as usize;
            let offset = (base as *const u8).align_offset(layout.align());
            assert!(offset < layout.align());
            (base + offset) as *mut ()
        };
        let slice = ptr::slice_from_raw_parts_mut(ptr as *mut u8, layout.size());
        unsafe { Ok(NonNull::new_unchecked(slice)) }
    }

    unsafe fn deallocate(&self, _ptr: NonNull<u8>, _layout: Layout) {}
}

#[cfg(test)]
mod tests {
    use super::*;

    fn data_ptr(slice: NonNull<[u8]>) -> NonNull<u8> {
        unsafe { NonNull::new_unchecked(slice.as_ptr() as *mut u8) }
    }

    #[test]
    fn test_allocation_backing_store_is_16_aligned() {
        assert_eq!(mem::align_of::<u128>(), ALLOCATION_ALIGNMENT);
    }

    #[test]
    fn test_allocate_returns_allocation_aligned() {
        let mut _object_allocator = ObjectAllocator::new();
        let allocator = _object_allocator.ptr();
        let layout = Layout::from_size_align(24, 8).unwrap();
        let ptr = Allocator::allocate(&allocator, layout).unwrap();
        assert_eq!(ptr.len(), 24);
        assert_eq!(data_ptr(ptr).as_ptr() as usize % ALLOCATION_ALIGNMENT, 0);
    }

    #[test]
    fn test_allocate_within_allocation_alignment() {
        let mut _object_allocator = ObjectAllocator::new();
        let allocator = _object_allocator.ptr();
        let layout = Layout::from_size_align(32, 16).unwrap();
        let ptr = Allocator::allocate(&allocator, layout).unwrap();
        assert_eq!(ptr.len(), 32);
        assert_eq!(data_ptr(ptr).as_ptr() as usize % ALLOCATION_ALIGNMENT, 0);
    }

    #[test]
    fn test_allocate_above_allocation_alignment() {
        let mut _object_allocator = ObjectAllocator::new();
        let allocator = _object_allocator.ptr();
        let layout = Layout::from_size_align(64, 32).unwrap();
        let ptr = Allocator::allocate(&allocator, layout).unwrap();
        assert_eq!(ptr.len(), 64);
        assert_eq!(data_ptr(ptr).as_ptr() as usize % 32, 0);
    }

    #[test]
    fn test_deallocate_does_nothing() {
        let mut _object_allocator = ObjectAllocator::new();
        let allocator = _object_allocator.ptr();
        let layout = Layout::from_size_align(16, 8).unwrap();
        let ptr = Allocator::allocate(&allocator, layout).unwrap();
        unsafe {
            Allocator::deallocate(&allocator, data_ptr(ptr), layout);
        }
    }

    #[test]
    fn test_grow() {
        let mut _object_allocator = ObjectAllocator::new();
        let allocator = _object_allocator.ptr();
        let old_layout = Layout::from_size_align(16, 8).unwrap();
        let new_layout = Layout::from_size_align(64, 8).unwrap();
        let old = Allocator::allocate(&allocator, old_layout).unwrap();
        unsafe {
            data_ptr(old).as_ptr().write_bytes(0xab, old_layout.size());
            let grown = Allocator::grow(&allocator, data_ptr(old), old_layout, new_layout).unwrap();
            assert_eq!(grown.len(), 64);
            let bytes = std::slice::from_raw_parts(data_ptr(grown).as_ptr(), old_layout.size());
            assert!(bytes.iter().all(|&b| b == 0xab));
        }
    }
}
