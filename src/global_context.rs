use std::sync::{Arc, Mutex, MutexGuard};

use super::ObjectAllocator;

pub struct GlobalContext {
    created_light_weight_thread_count: usize,
    allocator: Box<dyn ObjectAllocator>,
}

impl GlobalContext {
    fn new(allocator: Box<dyn ObjectAllocator>) -> Self {
        GlobalContext {
            created_light_weight_thread_count: 0,
            allocator,
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

pub fn create_global_context(allocator: Box<dyn ObjectAllocator>) -> GlobalContextPtr {
    let global_context = Arc::new(Mutex::new(GlobalContext::new(allocator)));
    GlobalContextPtr::from(global_context)
}
