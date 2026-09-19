use std::collections::VecDeque;
use std::sync::{Arc, Mutex, MutexGuard};

use crate::LightWeightThreadContext;
use crate::ObjectAllocator;
use crate::ObjectAllocatorPtr;

pub struct GlobalContext {
    created_light_weight_thread_count: usize,
    allocator: ObjectAllocator,
    run_queue: VecDeque<LightWeightThreadContext>,
    coros: Vec<Option<LightWeightThreadContext>>,
}

impl GlobalContext {
    fn new(allocator: ObjectAllocator) -> Self {
        GlobalContext {
            created_light_weight_thread_count: 0,
            allocator,
            run_queue: VecDeque::new(),
            coros: Vec::new(),
        }
    }

    pub fn issue_light_weight_thread_id(&mut self) -> usize {
        let id = self.created_light_weight_thread_count;
        self.created_light_weight_thread_count += 1;
        id
    }

    pub fn allocator(&mut self) -> ObjectAllocatorPtr {
        self.allocator.ptr()
    }

    pub(crate) fn run_gc(&mut self, running: &LightWeightThreadContext) {
        let allocator = self.allocator();
        let mut contexts: Vec<&LightWeightThreadContext> =
            Vec::with_capacity(1 + self.run_queue.len() + self.coros.len());
        contexts.push(running);
        contexts.extend(self.run_queue.iter());
        contexts.extend(self.coros.iter().flatten());
        allocator.run_gc(&contexts);
    }

    pub fn push_light_weight_thread(&mut self, ctx: LightWeightThreadContext) {
        self.run_queue.push_back(ctx)
    }

    pub fn pop_light_weight_thread(&mut self) -> Option<LightWeightThreadContext> {
        self.run_queue.pop_front()
    }

    pub fn reserve_coro_slot(&mut self) -> usize {
        self.coros.push(None);
        self.coros.len() - 1
    }

    pub fn park_coro(&mut self, slot: usize, ctx: LightWeightThreadContext) {
        let parked = self
            .coros
            .get_mut(slot)
            .unwrap_or_else(|| panic!("invalid coro slot: {}", slot));
        assert!(parked.is_none(), "coro slot {} already occupied", slot);
        *parked = Some(ctx);
    }

    pub fn unpark_coro(&mut self, slot: usize) -> LightWeightThreadContext {
        let mut ctx = self
            .coros
            .get_mut(slot)
            .unwrap_or_else(|| panic!("invalid coro slot: {}", slot))
            .take()
            .unwrap_or_else(|| panic!("coroswitch on empty coro slot: {}", slot));
        ctx.take_coro_slot();
        ctx
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
        let arc_ptr = std::mem::ManuallyDrop::new(arc_ptr);
        let ret = procedure(arc_ptr.lock().unwrap());
        let raw_ptr2 = Arc::into_raw(std::mem::ManuallyDrop::into_inner(arc_ptr));
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
pub fn create_global_context(allocator: ObjectAllocator) -> GlobalContextPtr {
    let global_context = Arc::new(Mutex::new(GlobalContext::new(allocator)));
    GlobalContextPtr::from(global_context)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::FunctionObject;
    use crate::ObjectAllocator;
    use crate::allocator::MAX_TOTAL_ALLOCATED_SIZE;
    use crate::create_light_weight_thread_context;
    use crate::type_id::TypeId;
    use std::mem;
    use std::ptr;

    fn make_gc() -> GlobalContextPtr {
        create_global_context(ObjectAllocator::new())
    }

    #[test]
    fn test_allocate_runs_gc_when_memory_exhausted() {
        let gc = make_gc();
        let mut ctx =
            create_light_weight_thread_context(gc.dupulicate(), FunctionObject::new_null());
        let (start, _end) = ctx.stack_range();
        ctx.grow_stack(mem::size_of::<usize>());
        let root = start as *mut usize;
        let kept = ctx.allocate(64, TypeId::new_invalid());
        assert!(!kept.is_null());
        unsafe {
            ptr::write(root, kept as usize);
            ptr::write(kept as *mut u8, 0xAB);
        }
        for _ in 0..10 {
            let ptr = ctx.allocate(65536, TypeId::new_invalid());
            assert!(!ptr.is_null());
        }
        let alive = gc.process(|mut gc| gc.allocator().contains(kept));
        assert!(alive);
        let total = gc.process(|mut gc| gc.allocator().total_size());
        assert!(total >= 64);
        assert!(total <= MAX_TOTAL_ALLOCATED_SIZE);
        unsafe {
            assert_eq!(ptr::read(kept as *const u8), 0xAB);
        }
    }

    #[test]
    #[should_panic(expected = "even after garbage collection")]
    fn test_allocate_panics_when_gc_does_not_free_enough() {
        let gc = make_gc();
        let mut ctx =
            create_light_weight_thread_context(gc.dupulicate(), FunctionObject::new_null());
        ctx.allocate(MAX_TOTAL_ALLOCATED_SIZE + 1, TypeId::new_invalid());
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
