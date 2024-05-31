use std::mem;
use std::ptr;
use std::slice;

use crate::defer_stack::DeferStack;
use crate::global_context::GlobalContextPtr;
use crate::object::interface::Interface;
use crate::FunctionObject;
use crate::StackFrame;
use crate::StackFrameCommon;
use crate::UserFunction;

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
        }
    }

    pub(crate) fn call<T>(
        &mut self,
        result_size: usize,
        args: &[*const ()],
        resume_func: FunctionObject,
    ) {
        let result_size = result_size.next_multiple_of(mem::size_of::<*const ()>());

        let p = self.stack_pointer as *mut u8;
        let p = unsafe { p.add(mem::size_of::<T>()) };
        let result_pointer = p as *const ();

        let p = unsafe { p.add(result_size) };
        let next_stack_pointer = p as *mut StackFrame;
        let next_frame = unsafe { &mut (*next_stack_pointer) };

        next_frame.common.resume_func = resume_func;
        next_frame.common.prev_stack_pointer = self.stack_pointer;
        next_frame.common.free_vars = ptr::null_mut();
        next_frame.common.defer_stack = DeferStack::new();

        let has_result = result_size > 0;
        let params_offset = usize::from(has_result);
        let additional_words = unsafe {
            slice::from_raw_parts_mut(
                next_frame.additional_words.as_mut_ptr(),
                params_offset + args.len(),
            )
        };

        if has_result {
            additional_words[0] = result_pointer;
        }

        let params = &mut additional_words[params_offset..];
        params.copy_from_slice(args);

        self.stack_pointer = next_stack_pointer;
    }

    pub(crate) fn leave(&mut self) -> FunctionObject {
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
            unsafe {
                let words = slice::from_raw_parts_mut(self.stack_pointer as *mut *mut (), 3);
                words[2] = object_ptrs; // free_vars
            }
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
}
