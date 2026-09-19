use std::collections::BTreeMap;
use std::ffi;
use std::mem;
use std::ptr;
use std::ptr::NonNull;

use allocator_api2::alloc::{AllocError, Allocator, Layout};

use crate::light_weight_thread::LightWeightThreadContext;
use crate::type_id::TypeId;

pub(crate) struct ObjectAllocator(ObjectAllocatorPtr);

impl ObjectAllocator {
    pub(crate) fn new() -> Self {
        ObjectAllocator(ObjectAllocatorPtr(Box::into_raw(Box::new(
            ObjectAllocatorInner::new(),
        ))))
    }

    pub(crate) fn ptr(&self) -> ObjectAllocatorPtr {
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
    global_spans: Vec<Span>,
    total_size: usize,
}

#[derive(Clone, Copy)]
struct Span {
    ptr: *mut (),
    size: usize,
}

struct AllocatedObject {
    span: Span,
    kind: AllocationKind,
    marked: bool,
    #[allow(dead_code)]
    type_id: TypeId,
}

#[derive(Clone, Copy, PartialEq)]
enum AllocationKind {
    Heap,
    GuardedPages,
}

const ALLOCATION_ALIGNMENT: usize = mem::size_of::<u128>();
pub(crate) const MAX_TOTAL_ALLOCATED_SIZE: usize = 1 << 20;

impl ObjectAllocatorInner {
    fn new() -> Self {
        ObjectAllocatorInner {
            allocated_objects: BTreeMap::new(),
            global_spans: Vec::new(),
            total_size: 0,
        }
    }

    fn allocate(&mut self, size: usize, type_id: TypeId) -> *mut () {
        let size = size.div_ceil(ALLOCATION_ALIGNMENT) * ALLOCATION_ALIGNMENT;
        if self.total_size.saturating_add(size) > MAX_TOTAL_ALLOCATED_SIZE {
            return ptr::null_mut();
        }
        let mut buf: Vec<u128> = Vec::new();
        if buf.try_reserve_exact(size / ALLOCATION_ALIGNMENT).is_err() {
            return ptr::null_mut();
        }
        buf.resize(size / ALLOCATION_ALIGNMENT, 0);
        let ptr = buf.leak().as_mut_ptr() as *mut ();
        self.allocated_objects.insert(
            ptr as usize,
            AllocatedObject {
                span: Span { ptr, size },
                kind: AllocationKind::Heap,
                marked: false,
                type_id,
            },
        );
        self.total_size += size;
        ptr
    }

    fn allocate_guarded_pages(&mut self, num_pages: usize, type_id: TypeId) -> *mut () {
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
                    span: Span {
                        ptr,
                        size: 4096 * num_pages,
                    },
                    kind: AllocationKind::GuardedPages,
                    marked: false,
                    type_id,
                },
            );
            ptr
        }
    }

    fn free(&mut self, object: &AllocatedObject) {
        match object.kind {
            AllocationKind::Heap => {
                self.total_size = self
                    .total_size
                    .checked_sub(object.span.size)
                    .expect("total allocated size underflow while freeing an object");
                unsafe {
                    Vec::from_raw_parts(
                        object.span.ptr as *mut u128,
                        0,
                        object.span.size / ALLOCATION_ALIGNMENT,
                    );
                }
            }
            AllocationKind::GuardedPages => {
                let base_addr = (object.span.ptr as usize - 4096) as *mut libc::c_void;
                unsafe {
                    libc::munmap(base_addr, object.span.size + 4096);
                }
            }
        }
    }

    fn sweep(&mut self) {
        let objects = mem::take(&mut self.allocated_objects);
        let kept = objects
            .into_iter()
            .filter_map(|(address, object)| {
                if object.marked {
                    Some((address, object))
                } else {
                    self.free(&object);
                    None
                }
            })
            .collect();
        self.allocated_objects = kept;
    }

    fn register_global_object(&mut self, address: *mut (), size: usize) {
        self.global_spans.push(Span { ptr: address, size });
    }

    fn free_all_allocated_objects(&mut self) {
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        self.sweep();
        assert!(
            self.total_size == 0,
            "total allocated size {} did not reach zero after freeing all objects",
            self.total_size
        );
    }

    pub(crate) fn run_gc(&mut self, contexts: &[&LightWeightThreadContext]) {
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        let global_spans = mem::take(&mut self.global_spans);
        for span in &global_spans {
            self.mark_range(span.ptr as usize, span.ptr as usize + span.size);
        }
        self.global_spans = global_spans;
        for context in contexts {
            let (start, end) = context.stack_range();
            self.mark_range(start as usize, end as usize);
        }
        self.sweep();
    }

    fn mark_range(&mut self, start: usize, end: usize) {
        let word_size = mem::size_of::<usize>();
        let mut address = start;
        while address + word_size <= end {
            let word = unsafe { ptr::read_unaligned(address as *const usize) };
            if let Some(object_address) = self.containing_object(word) {
                self.mark_object(object_address);
            }
            address += word_size;
        }
    }

    fn mark_object(&mut self, object_address: usize) {
        let (span, kind, marked) = match self.allocated_objects.get(&object_address) {
            Some(object) => (object.span, object.kind, object.marked),
            None => return,
        };
        if marked {
            return;
        }
        self.allocated_objects
            .get_mut(&object_address)
            .unwrap()
            .marked = true;
        if kind == AllocationKind::Heap {
            self.mark_range(span.ptr as usize, span.ptr as usize + span.size);
        }
    }

    fn containing_object(&self, address: usize) -> Option<usize> {
        let (object_address, object) = self.allocated_objects.range(..=address).next_back()?;
        let object_address = *object_address;
        if address < object_address + object.span.size {
            Some(object_address)
        } else {
            None
        }
    }
}

#[derive(Clone)]
pub(crate) struct ObjectAllocatorPtr(*mut ObjectAllocatorInner);

impl ObjectAllocatorPtr {
    pub(crate) fn allocate(&self, size: usize, type_id: TypeId) -> *mut () {
        unsafe { &mut *self.0 }.allocate(size, type_id)
    }

    pub(crate) fn allocate_guarded_pages(&self, num_pages: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate_guarded_pages(num_pages, TypeId::new_invalid())
    }

    pub(crate) fn register_global_object(&self, address: *mut (), size: usize) {
        unsafe { &mut *self.0 }.register_global_object(address, size);
    }

    pub(crate) fn run_gc(&self, contexts: &[&LightWeightThreadContext]) {
        unsafe { &mut *self.0 }.run_gc(contexts)
    }

    #[cfg(test)]
    pub(crate) fn global_spans_len(&self) -> usize {
        unsafe { &*self.0 }.global_spans.len()
    }

    #[cfg(test)]
    pub(crate) fn total_size(&self) -> usize {
        unsafe { &*self.0 }.total_size
    }

    #[cfg(test)]
    pub(crate) fn contains(&self, ptr: *mut ()) -> bool {
        unsafe { &*self.0 }
            .allocated_objects
            .contains_key(&(ptr as usize))
    }
}

unsafe impl Allocator for ObjectAllocatorPtr {
    fn allocate(&self, layout: Layout) -> Result<NonNull<[u8]>, AllocError> {
        let ptr = if layout.align() <= ALLOCATION_ALIGNMENT {
            let ptr = self.allocate(layout.size(), TypeId::new_invalid());
            if ptr.is_null() {
                return Err(AllocError);
            }
            ptr
        } else {
            let total = layout.size() + layout.align();
            let base = self.allocate(total, TypeId::new_invalid());
            if base.is_null() {
                return Err(AllocError);
            }
            let base = base as usize;
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
    fn test_register_global_object() {
        let mut inner = ObjectAllocatorInner::new();
        let mut data = [0u8; 24];
        let address = data.as_mut_ptr() as *mut ();
        inner.register_global_object(address, data.len());
        assert_eq!(inner.global_spans.len(), 1);
        let registered = &inner.global_spans[0];
        assert_eq!(registered.ptr, address);
        assert_eq!(registered.size, data.len());
        assert!(inner.allocated_objects.is_empty());
    }

    #[test]
    fn test_sweep_frees_unmarked_objects() {
        let mut inner = ObjectAllocatorInner::new();
        let marked_ptr = inner.allocate(16, TypeId::new_invalid());
        let unmarked_ptr = inner.allocate(16, TypeId::new_invalid());
        assert!(!marked_ptr.is_null());
        assert!(!unmarked_ptr.is_null());
        inner
            .allocated_objects
            .get_mut(&(marked_ptr as usize))
            .unwrap()
            .marked = true;
        inner.sweep();
        assert!(inner.allocated_objects.contains_key(&(marked_ptr as usize)));
        assert!(
            !inner
                .allocated_objects
                .contains_key(&(unmarked_ptr as usize))
        );
        inner.free_all_allocated_objects();
    }

    #[test]
    fn test_free_all_allocated_objects_clears_marks() {
        let mut inner = ObjectAllocatorInner::new();
        let ptr = inner.allocate(16, TypeId::new_invalid());
        inner
            .allocated_objects
            .get_mut(&(ptr as usize))
            .unwrap()
            .marked = true;
        inner.free_all_allocated_objects();
        assert!(inner.allocated_objects.is_empty());
    }

    #[test]
    fn test_total_size_tracks_allocate_and_free() {
        let mut inner = ObjectAllocatorInner::new();
        let a = inner.allocate(16, TypeId::new_invalid());
        let b = inner.allocate(32, TypeId::new_invalid());
        assert!(!a.is_null());
        assert!(!b.is_null());
        assert_eq!(inner.total_size, 16 + 32);
        inner
            .allocated_objects
            .get_mut(&(a as usize))
            .unwrap()
            .marked = true;
        inner.sweep();
        assert_eq!(inner.total_size, 16);
        inner.free_all_allocated_objects();
        assert_eq!(inner.total_size, 0);
    }

    #[test]
    fn test_allocate_returns_null_above_limit() {
        let mut inner = ObjectAllocatorInner::new();
        while !inner.allocate(65536, TypeId::new_invalid()).is_null() {}
        assert!(inner.total_size <= MAX_TOTAL_ALLOCATED_SIZE);
        assert!(inner.allocate(65536, TypeId::new_invalid()).is_null());
        inner.free_all_allocated_objects();
        assert_eq!(inner.total_size, 0);
    }

    #[test]
    fn test_mark_marks_reachable_digraph() {
        let mut inner = ObjectAllocatorInner::new();
        let root = Box::into_raw(Box::new(0usize));
        inner.register_global_object(root as *mut (), mem::size_of::<usize>());
        let a = inner.allocate(64, TypeId::new_invalid());
        let b = inner.allocate(64, TypeId::new_invalid());
        let c = inner.allocate(64, TypeId::new_invalid());
        let garbage = inner.allocate(64, TypeId::new_invalid());
        unsafe {
            root.write(a as usize);
            ptr::write(a as *mut usize, b as usize);
            ptr::write(b as *mut usize, c as usize);
        }
        inner.run_gc(&[]);
        assert!(inner.allocated_objects.contains_key(&(a as usize)));
        assert!(inner.allocated_objects.contains_key(&(b as usize)));
        assert!(inner.allocated_objects.contains_key(&(c as usize)));
        assert!(!inner.allocated_objects.contains_key(&(garbage as usize)));
        inner.free_all_allocated_objects();
        unsafe {
            let _ = Box::from_raw(root);
        }
    }

    #[test]
    fn test_mark_scans_context_stack_range() {
        let mut inner = ObjectAllocatorInner::new();
        let kept = inner.allocate(64, TypeId::new_invalid());
        let garbage = inner.allocate(64, TypeId::new_invalid());
        let gc = crate::global_context::create_global_context(crate::ObjectAllocator::new());
        let mut ctx = crate::create_light_weight_thread_context(
            gc.dupulicate(),
            crate::FunctionObject::new_null(),
        );
        let (start, _end) = ctx.stack_range();
        ctx.grow_stack(mem::size_of::<usize>());
        let address = start as *mut usize;
        unsafe {
            ptr::write(address, kept as usize);
        }
        inner.run_gc(&[&ctx]);
        assert!(inner.allocated_objects.contains_key(&(kept as usize)));
        assert!(!inner.allocated_objects.contains_key(&(garbage as usize)));
        inner.free_all_allocated_objects();
    }

    #[test]
    fn test_mark_ignores_one_past_the_end_pointer() {
        let mut inner = ObjectAllocatorInner::new();
        let a = inner.allocate(16, TypeId::new_invalid());
        let root_holding = Box::into_raw(Box::new(a as usize + 16));
        inner.register_global_object(root_holding as *mut (), mem::size_of::<usize>());
        inner.run_gc(&[]);
        assert!(!inner.allocated_objects.contains_key(&(a as usize)));
        inner.free_all_allocated_objects();
        unsafe {
            let _ = Box::from_raw(root_holding);
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
