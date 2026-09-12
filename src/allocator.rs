use std::collections::BTreeMap;
use std::ffi;
use std::mem;
use std::ptr;

pub(crate) struct ObjectAllocator {
    allocated_objects: BTreeMap<usize, AllocatedObject>,
}

struct AllocatedObject {
    ptr: *mut (),
    size: usize,
    destructor: fn(*mut ()),
    kind: AllocationKind,
}

#[derive(Clone, Copy)]
enum AllocationKind {
    Heap,
    GuardedPages,
}

impl ObjectAllocator {
    pub(crate) fn new() -> Self {
        ObjectAllocator {
            allocated_objects: BTreeMap::new(),
        }
    }

    pub(crate) fn ptr(&mut self) -> ObjectAllocatorPtr {
        ObjectAllocatorPtr(self as *mut ObjectAllocator)
    }

    pub(crate) fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
        let alignment = mem::size_of::<isize>();
        let size = size.div_ceil(alignment) * alignment;
        let buf: Vec<isize> = vec![0; size];
        let ptr = buf.leak().as_mut_ptr();
        let ptr = ptr as *mut ();
        self.allocated_objects.insert(
            ptr as usize,
            AllocatedObject {
                ptr,
                size,
                destructor,
                kind: AllocationKind::Heap,
            },
        );
        ptr
    }

    pub(crate) fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
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
                    destructor: |_| {},
                    kind: AllocationKind::GuardedPages,
                },
            );
            ptr
        }
    }
}

impl Drop for ObjectAllocator {
    fn drop(&mut self) {
        for object in self.allocated_objects.values() {
            (object.destructor)(object.ptr);
            match object.kind {
                AllocationKind::Heap => unsafe {
                    Vec::from_raw_parts(object.ptr as *mut isize, 0, object.size);
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
pub(crate) struct ObjectAllocatorPtr(*mut ObjectAllocator);

impl ObjectAllocatorPtr {
    pub(crate) fn allocate(&self, size: usize, destructor: fn(*mut ())) -> *mut () {
        unsafe { &mut *self.0 }.allocate(size, destructor)
    }

    pub(crate) fn allocate_guarded_pages(&self, num_pages: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate_guarded_pages(num_pages)
    }
}
