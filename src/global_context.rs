use std::collections::VecDeque;
use std::sync::{Arc, Mutex, MutexGuard};

use crate::LightWeightThreadContext;
use crate::ObjectAllocator;

pub struct GlobalContext {
    created_light_weight_thread_count: usize,
    allocator: Box<dyn ObjectAllocator>,
    run_queue: VecDeque<LightWeightThreadContext>,
}

impl GlobalContext {
    fn new(allocator: Box<dyn ObjectAllocator>) -> Self {
        GlobalContext {
            created_light_weight_thread_count: 0,
            allocator,
            run_queue: VecDeque::new(),
        }
    }

    pub fn issue_light_weight_thread_id(&mut self) -> usize {
        let id = self.created_light_weight_thread_count;
        self.created_light_weight_thread_count += 1;
        id
    }

    pub fn allocator(&mut self) -> &mut dyn ObjectAllocator {
        &mut *self.allocator
    }

    pub fn push_light_weight_thread(&mut self, ctx: LightWeightThreadContext) {
        self.run_queue.push_back(ctx)
    }

    pub fn pop_light_weight_thread(&mut self) -> Option<LightWeightThreadContext> {
        self.run_queue.pop_front()
    }
}

#[derive(PartialEq, Eq, Debug)]
#[repr(C)]
pub struct GlobalContextPtr(*const ());

impl GlobalContextPtr {
    pub fn from(arc_ptr: Arc<Mutex<GlobalContext>>) -> Self {
        GlobalContextPtr(Arc::into_raw(arc_ptr) as *const ())
    }

    pub fn dupulicate(&self) -> Self {
        let raw_ptr = self.0 as *const Mutex<GlobalContext>;
        let arc_ptr = unsafe { Arc::from_raw(raw_ptr) };
        let arc_ptr2 = arc_ptr.clone();
        let raw_ptr2 = Arc::into_raw(arc_ptr);
        assert_eq!(raw_ptr, raw_ptr2);
        GlobalContextPtr::from(arc_ptr2)
    }

    pub fn process<F, T>(&self, procedure: F) -> T
    where
        F: FnOnce(MutexGuard<GlobalContext>) -> T,
    {
        let raw_ptr = self.0 as *const Mutex<GlobalContext>;
        let arc_ptr = unsafe { Arc::from_raw(raw_ptr) };
        let ret = procedure(arc_ptr.lock().unwrap());
        let raw_ptr2 = Arc::into_raw(arc_ptr);
        assert_eq!(raw_ptr, raw_ptr2);
        ret
    }
}

impl Drop for GlobalContextPtr {
    fn drop(&mut self) {
        let raw_ptr = self.0 as *const Mutex<GlobalContext>;
        unsafe { Arc::from_raw(raw_ptr) };
    }
}

// ToDo: fix
#[allow(clippy::arc_with_non_send_sync)]
pub fn create_global_context(allocator: Box<dyn ObjectAllocator>) -> GlobalContextPtr {
    let global_context = Arc::new(Mutex::new(GlobalContext::new(allocator)));
    GlobalContextPtr::from(global_context)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;

    struct MockObjectAllocator {
        allocated: Vec<(*mut (), usize)>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, _destructor: fn(*mut ())) -> *mut () {
            let alignment = std::mem::align_of::<isize>();
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

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for (ptr, size) in &self.allocated {
                unsafe {
                    let _ = Vec::from_raw_parts(*ptr as *mut isize, 0, *size);
                }
            }
        }
    }

    fn make_gc() -> GlobalContextPtr {
        create_global_context(Box::new(MockObjectAllocator::new()))
    }

    #[test]
    fn test_issue_light_weight_thread_id_increments() {
        let gc = make_gc();
        gc.process(|mut gc| {
            let id0 = gc.issue_light_weight_thread_id();
            let id1 = gc.issue_light_weight_thread_id();
            let id2 = gc.issue_light_weight_thread_id();
            assert_eq!(id0, 0);
            assert_eq!(id1, 1);
            assert_eq!(id2, 2);
        });
    }

    #[test]
    fn test_global_context_ptr_process() {
        let gc = make_gc();
        let result = gc.process(|gc| {
            drop(gc);
            42
        });
        assert_eq!(result, 42);
    }

    #[test]
    fn test_global_context_ptr_duplicate() {
        let gc = make_gc();
        let gc2 = gc.dupulicate();
        assert_eq!(gc, gc2);
    }

    #[test]
    fn test_push_pop_light_weight_thread() {
        let gc = make_gc();
        gc.process(|mut gc| {
            assert!(gc.pop_light_weight_thread().is_none());
        });
    }

    #[test]
    fn test_create_global_context() {
        let _gc = make_gc();
    }

    #[test]
    fn test_allocator_access() {
        let gc = make_gc();
        gc.process(|mut gc| {
            let _alloc = gc.allocator();
        });
    }
}
