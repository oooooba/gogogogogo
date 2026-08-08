use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::StackFrame;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::defer_stack::DeferStack;
use crate::global_context::GlobalContextPtr;
use crate::object::interface::Interface;

#[repr(C)]
pub struct LightWeightThreadContext {
    stack_pointer: *mut StackFrame,
    prev_func: UserFunction,
    marker: isize,
    id: usize,
    global_context: GlobalContextPtr,
    current_func: FunctionObject,
    control_flags: usize,
    panic_data: Interface,
    initial_stack_pointer: *mut StackFrame,
    coro_slot: Option<usize>,
}

impl LightWeightThreadContext {
    pub(crate) fn new(
        id: usize,
        global_context: GlobalContextPtr,
        stack_pointer: *mut StackFrame,
        entry_func: FunctionObject,
        prev_func: UserFunction,
    ) -> Self {
        LightWeightThreadContext {
            stack_pointer,
            prev_func,
            marker: 0xdeadbeef,
            id,
            global_context,
            current_func: entry_func,
            control_flags: 0,
            panic_data: Interface::nil(),
            initial_stack_pointer: stack_pointer,
            coro_slot: None,
        }
    }

    pub(crate) fn grow_stack(&mut self, size: usize) {
        let size = size.next_multiple_of(mem::size_of::<*const ()>());
        let p = self.stack_pointer as *mut u8;
        self.stack_pointer = unsafe { p.add(size) } as *mut StackFrame;
    }

    pub(crate) fn push_frame(
        &mut self,
        prev_stack_pointer: *mut StackFrame,
        result_pointer: Option<*const ()>,
        args: &[*const ()],
        resume_func: FunctionObject,
    ) {
        let next_stack_pointer = self.stack_pointer;
        let next_frame = unsafe { &mut (*next_stack_pointer) };

        next_frame.common.resume_func = resume_func;
        next_frame.common.prev_stack_pointer = prev_stack_pointer;
        next_frame.common.free_vars = ptr::null_mut();
        next_frame.common.defer_stack = DeferStack::new();

        let params_offset = usize::from(result_pointer.is_some());
        let base =
            unsafe { ptr::addr_of_mut!((*next_stack_pointer).additional_words) as *mut *const () };
        if let Some(result_pointer) = result_pointer {
            unsafe {
                ptr::write(base, result_pointer);
            }
        }

        unsafe {
            let dst = base.add(params_offset);
            ptr::copy_nonoverlapping(args.as_ptr(), dst, args.len());
        }

        self.stack_pointer = next_stack_pointer;
    }

    pub(crate) fn pop_frame(&mut self) -> FunctionObject {
        let (prev_stack_pointer, resume_func) = {
            let stack_frame = self.stack_frame::<StackFrameCommon>();
            (
                stack_frame.prev_stack_pointer,
                stack_frame.resume_func.clone(),
            )
        };
        self.stack_pointer = prev_stack_pointer;
        resume_func
    }

    pub(crate) fn prepare_user_function(&mut self) -> UserFunction {
        let (func, object_ptrs) = self.current_func.extract_user_function();
        if let Some(object_ptrs) = object_ptrs {
            self.stack_frame_mut::<StackFrameCommon>().free_vars = object_ptrs;
        }
        func
    }

    pub(crate) fn update_current_func(&mut self, func: FunctionObject) {
        self.prev_func = self.current_func.extract_user_function().0;
        self.current_func = func
    }

    pub(crate) fn id(&self) -> usize {
        self.id
    }

    pub(crate) fn is_main(&self) -> bool {
        self.id == 0
    }

    pub(crate) fn global_context(&self) -> &GlobalContextPtr {
        &self.global_context
    }

    pub(crate) fn stack_pointer(&self) -> *mut StackFrame {
        self.stack_pointer
    }

    pub(crate) fn stack_frame<T>(&self) -> &T {
        let p = self.stack_pointer as *const T;
        unsafe { &*p }
    }

    pub(crate) fn stack_frame_mut<T>(&mut self) -> &mut T {
        let p = self.stack_pointer as *mut T;
        unsafe { &mut *p }
    }

    pub(crate) fn is_stack_empty(&self) -> bool {
        assert!(self.initial_stack_pointer <= self.stack_pointer);
        self.initial_stack_pointer == self.stack_pointer
    }

    pub(crate) fn suspend(&mut self) {
        self.control_flags |= 0b1;
    }

    pub(crate) fn resume(&mut self) {
        self.control_flags &= !0b1;
    }

    pub(crate) fn is_suspended(&self) -> bool {
        self.control_flags & 0b1 > 0
    }

    pub(crate) fn terminate(&mut self) {
        self.control_flags |= 0b10;
    }

    pub(crate) fn is_terminated(&self) -> bool {
        self.control_flags & 0b10 > 0
    }

    pub(crate) fn enter_panic(&mut self, data: Interface) {
        self.control_flags |= 0b100;
        self.panic_data = data;
    }

    pub(crate) fn exit_panic(&mut self) -> Interface {
        assert!(self.is_panicking());
        self.control_flags &= !0b100;
        self.panic_data.clone()
    }

    pub(crate) fn is_panicking(&self) -> bool {
        self.control_flags & 0b100 > 0
    }

    pub(crate) fn set_coro_slot(&mut self, slot: Option<usize>) {
        self.coro_slot = slot;
    }

    pub(crate) fn take_coro_slot(&mut self) -> Option<usize> {
        self.coro_slot.take()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;

    struct AllocatedObject {
        ptr: *mut (),
        size: usize,
        destructor: fn(*mut ()),
    }

    struct MockObjectAllocator {
        allocated_objects: Vec<AllocatedObject>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated_objects: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = size.div_ceil(alignment) * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
            self.allocate(num_pages * 4096, |_| {})
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for obj in &self.allocated_objects {
                (obj.destructor)(obj.ptr);
                unsafe {
                    Vec::from_raw_parts(obj.ptr as *mut isize, 0, obj.size);
                }
            }
        }
    }

    fn create_ctx() -> (LightWeightThreadContext, crate::GlobalContextPtr) {
        let allocator = Box::new(MockObjectAllocator::new());
        let gc = global_context::create_global_context(allocator);
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    #[test]
    fn test_grow_stack_advances_pointer() {
        let (mut ctx, _gc) = create_ctx();
        let sp_before = ctx.stack_pointer();
        ctx.grow_stack(64);
        let sp_after = ctx.stack_pointer();
        assert!(sp_after as usize > sp_before as usize);
        assert_eq!((sp_after as usize) - (sp_before as usize), 64);
    }

    #[test]
    fn test_grow_stack_aligns_to_word() {
        let (mut ctx, _gc) = create_ctx();
        let sp_before = ctx.stack_pointer();
        ctx.grow_stack(3);
        let sp_after = ctx.stack_pointer();
        let diff = (sp_after as usize) - (sp_before as usize);
        assert_eq!(diff % mem::size_of::<*const ()>(), 0);
    }

    #[test]
    fn test_push_pop_frame_roundtrip() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>() + 128);
        let resume = FunctionObject::new_null();
        ctx.push_frame(prev_sp, None, &[], resume.clone());
        let popped = ctx.pop_frame();
        assert_eq!(popped, resume);
        assert_eq!(ctx.stack_pointer(), prev_sp);
    }

    #[test]
    fn test_push_pop_frame_with_args() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>() + 128);
        let arg1 = 0xaaaa as *const ();
        let arg2 = 0xbbbb as *const ();
        ctx.push_frame(prev_sp, None, &[arg1, arg2], FunctionObject::new_null());
        let popped = ctx.pop_frame();
        assert_eq!(popped, FunctionObject::new_null());
        assert_eq!(ctx.stack_pointer(), prev_sp);
    }

    #[test]
    fn test_suspend_resume() {
        let (mut ctx, _gc) = create_ctx();
        assert!(!ctx.is_suspended());
        ctx.suspend();
        assert!(ctx.is_suspended());
        ctx.resume();
        assert!(!ctx.is_suspended());
    }

    #[test]
    fn test_terminate() {
        let (mut ctx, _gc) = create_ctx();
        assert!(!ctx.is_terminated());
        ctx.terminate();
        assert!(ctx.is_terminated());
    }

    #[test]
    fn test_panic_enter_exit() {
        let (mut ctx, _gc) = create_ctx();
        assert!(!ctx.is_panicking());
        let data = Interface::nil();
        ctx.enter_panic(data);
        assert!(ctx.is_panicking());
        let recovered = ctx.exit_panic();
        assert!(!ctx.is_panicking());
        assert!(recovered.is_nil());
    }

    #[test]
    fn test_stack_frame_read_write() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StackFrameCommon>() + 128);
        ctx.push_frame(prev_sp, None, &[], FunctionObject::new_null());
        {
            let frame = ctx.stack_frame::<StackFrameCommon>();
            assert!(!frame.prev_stack_pointer.is_null() || prev_sp == frame.prev_stack_pointer);
        }
        ctx.pop_frame();
    }

    #[test]
    fn test_global_context_access() {
        let (ctx, _gc) = create_ctx();
        let _ = ctx.global_context();
    }
}
