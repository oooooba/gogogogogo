use std::mem;
use std::ptr;

use crate::FunctionObject;
use crate::ObjectAllocatorPtr;
use crate::StackFrame;
use crate::StackFrameCommon;
use crate::UserFunction;
use crate::defer_stack::DeferStack;
use crate::global_context::GlobalContextPtr;
use crate::object::interface::Interface;
use crate::type_id::TypeId;

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

    /// Pushes a new frame at the current stack pointer. `frame_size` is the
    /// total extent of that frame, trailing argument buffer included, and is
    /// recorded so the GC can scan the frame conservatively when it carries no
    /// stack map of its own.
    pub(crate) fn push_frame(
        &mut self,
        frame_size: usize,
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
        // Frames pushed here are runtime frames: they carry no stack map, so
        // the GC scans them conservatively over `frame_size`.
        next_frame.common.frame_type = TypeId::new_invalid();
        next_frame.common.frame_size = frame_size;

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

    /// Total extent of a frame the runtime pushes itself: the fixed part, the
    /// buffer for the result, and the argument words that follow the result
    /// pointer. `result_size` is the size of the result buffer in bytes and may
    /// be zero when the frame has no result.
    pub(crate) fn frame_extent(fixed_size: usize, result_size: usize, args_len: usize) -> usize {
        let word_size = mem::size_of::<*const ()>();
        let args = word_size * args_len;
        if result_size > 0 {
            fixed_size + result_size.max(word_size + args)
        } else {
            fixed_size + args
        }
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

    pub(crate) fn allocate(&mut self, size: usize, type_id: TypeId) -> *mut () {
        self.allocate_with(size, move |allocator| allocator.allocate(size, type_id))
    }

    pub(crate) fn allocate_closure(&mut self, size: usize) -> *mut () {
        self.allocate_with(size, |allocator| allocator.allocate_closure(size))
    }

    pub(crate) fn allocate_slice_buffer(
        &mut self,
        size: usize,
        type_id: TypeId,
        accessible_bytes: usize,
    ) -> *mut () {
        self.allocate_with(size, move |allocator| {
            allocator.allocate_slice_buffer(size, type_id, accessible_bytes)
        })
    }

    pub(crate) fn set_slice_scan_end(&mut self, interior_addr: usize, absolute_end: usize) {
        self.global_context().process(|mut global_context| {
            global_context
                .allocator()
                .set_slice_scan_end(interior_addr, absolute_end);
        });
    }

    fn allocate_with(
        &mut self,
        size: usize,
        allocate: impl Fn(ObjectAllocatorPtr) -> *mut (),
    ) -> *mut () {
        self.global_context().process(|mut global_context| {
            let ptr = allocate(global_context.allocator());
            if !ptr.is_null() {
                return ptr;
            }
            global_context.run_gc(&*self);
            let ptr = allocate(global_context.allocator());
            if ptr.is_null() {
                panic!(
                    "out of memory: failed to allocate {} bytes even after garbage collection",
                    size
                );
            }
            ptr
        })
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

    /// Address of the bottom of this goroutine's stack region, which is also
    /// the start of the allocator object backing it.
    pub(crate) fn stack_base(&self) -> usize {
        self.initial_stack_pointer as usize
    }

    /// Iterates the live frames of this goroutine, innermost first.
    pub(crate) fn stack_frames(&self) -> StackFrames {
        StackFrames {
            base: self.stack_base(),
            next: self.stack_pointer as usize,
            finished: false,
        }
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

/// What the GC needs to know about one live stack frame.
pub(crate) struct StackFrameDescriptor {
    /// Address of the frame, i.e. of its StackFrameCommon.
    pub(crate) address: usize,
    /// Stack map source of the frame, invalid when the frame carries none.
    pub(crate) frame_type: TypeId,
    /// Total extent of the frame in bytes, 0 when unknown.
    pub(crate) frame_size: usize,
}

/// Iterator over the frames of a goroutine stack, innermost first. The bottom
/// frame links to itself, which terminates the walk.
pub(crate) struct StackFrames {
    base: usize,
    next: usize,
    finished: bool,
}

impl Iterator for StackFrames {
    type Item = StackFrameDescriptor;

    fn next(&mut self) -> Option<StackFrameDescriptor> {
        if self.finished || self.next < self.base {
            return None;
        }
        // Every frame starts with a StackFrameCommon, so reading it stays
        // inside the frame.
        let common = unsafe { &*(self.next as *const crate::StackFrameCommon) };
        let descriptor = StackFrameDescriptor {
            address: self.next,
            frame_type: common.frame_type,
            frame_size: common.frame_size,
        };
        // The bottom frame links to itself; anything else that does not point
        // further down ends the walk as well. The bottom frame sits exactly at
        // the stack base (a generated frame can occupy `base`, not `base + 8`),
        // so a `prev` equal to the base is a valid pointer, not a stray word;
        // only the `prev == self.next` self-link terminates the walk.
        let prev = common.prev_stack_pointer as usize;
        if prev >= self.base && prev != self.next {
            self.next = prev;
        } else {
            self.finished = true;
        }
        Some(descriptor)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;

    fn create_ctx() -> (LightWeightThreadContext, crate::GlobalContextPtr) {
        let gc = global_context::create_global_context(ObjectAllocator::new());
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
        ctx.push_frame(
            mem::size_of::<StackFrameCommon>() + 128,
            prev_sp,
            None,
            &[],
            resume.clone(),
        );
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
        ctx.push_frame(
            mem::size_of::<StackFrameCommon>() + 128,
            prev_sp,
            None,
            &[arg1, arg2],
            FunctionObject::new_null(),
        );
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
        ctx.push_frame(
            mem::size_of::<StackFrameCommon>() + 128,
            prev_sp,
            None,
            &[],
            FunctionObject::new_null(),
        );
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
