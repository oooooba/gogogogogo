mod api;
mod channel;
mod global_context;

use std::collections::VecDeque;
use std::ffi;
use std::mem;
use std::process;
use std::ptr;
use std::slice;

use api::{FunctionObject, StackFrame, UserFunction};
use global_context::GlobalContextPtr;

#[repr(C)]
pub struct LightWeightThreadContext {
    global_context: GlobalContextPtr,
    current_func: FunctionObject,
    stack_pointer: *mut StackFrame,
    prev_func: UserFunction,
    marker: isize,
}

impl LightWeightThreadContext {
    fn new(
        global_context: GlobalContextPtr,
        stack_pointer: *mut StackFrame,
        prev_func: UserFunction,
    ) -> Self {
        LightWeightThreadContext {
            global_context,
            current_func: FunctionObject::new_null(),
            stack_pointer,
            prev_func,
            marker: 0xdeadbeef,
        }
    }

    fn set_up(
        &mut self,
        entry_func: FunctionObject,
        result_size: usize,
        arg_buffer_ptr: ObjectPtr,
        num_arg_buffer_words: usize,
    ) {
        self.current_func = entry_func;
        unsafe {
            let result_pointer = self.stack_pointer();
            let next_stack_pointer =
                (result_pointer as *mut u8).add(result_size) as *mut StackFrame;
            self.update_stack_pointer(next_stack_pointer);
            let words = slice::from_raw_parts_mut(self.stack_frame_mut().words.as_mut_ptr(), 5);
            words[0] = terminate as *mut ();
            words[1] = ptr::null_mut();
            words[2] = ptr::null_mut();
            let mut arg_base = 3;
            if result_size > 0 {
                words[arg_base] = result_pointer;
                arg_base += 1;
            }
            let src_arg_buffer_ptr = arg_buffer_ptr.as_ref::<usize>();
            let dst_arg_buffer_ptr = &mut words[arg_base] as *mut *mut () as *mut usize;
            ptr::copy_nonoverlapping(src_arg_buffer_ptr, dst_arg_buffer_ptr, num_arg_buffer_words);
        }
    }

    fn prepare_user_function(&mut self) -> UserFunction {
        let (func, object_ptrs) = self.current_func.extrace_user_function();
        if let Some(object_ptrs) = object_ptrs {
            unsafe {
                let words = slice::from_raw_parts_mut(self.stack_frame_mut().words.as_mut_ptr(), 3);
                words[2] = object_ptrs; // free_vars
            }
        }
        func
    }

    fn global_context(&self) -> &GlobalContextPtr {
        &self.global_context
    }

    fn update_current_func(&mut self, func: FunctionObject) {
        self.prev_func = self.current_func.extrace_user_function().0;
        self.current_func = func
    }

    fn stack_pointer(&self) -> *mut () {
        self.stack_pointer as *mut ()
    }

    fn update_stack_pointer(&mut self, new_stack_pointer: *mut StackFrame) {
        self.stack_pointer = new_stack_pointer
    }

    fn stack_frame(&self) -> &StackFrame {
        unsafe { &*self.stack_pointer }
    }

    fn stack_frame_mut(&mut self) -> &mut StackFrame {
        unsafe { &mut *self.stack_pointer }
    }
}

struct LightWeightThread {
    context: LightWeightThreadContext,
    is_main: bool,
    is_running: bool,
}

impl LightWeightThread {
    fn new(context: LightWeightThreadContext) -> Self {
        LightWeightThread {
            context,
            is_main: false,
            is_running: false,
        }
    }

    fn is_main(&self) -> bool {
        self.is_main
    }

    fn set_main(&mut self) {
        self.is_main = true;
    }

    fn is_running(&self) -> bool {
        self.is_running
    }

    fn start(&mut self) {
        self.is_running = true;
    }

    fn execute(&mut self) -> Option<LightWeightThreadContext> {
        assert!(self.is_running);
        let mut new_ctx: Option<LightWeightThreadContext> = None;
        loop {
            let func = self.context.prepare_user_function();
            let (next_func, suspends) = if func == gox5_schedule {
                (api::schedule(&mut self.context), true)
            } else if func == gox5_send {
                (api::send(&mut self.context)?, false)
            } else if func == gox5_recv {
                (api::recv(&mut self.context)?, false)
            } else if func == gox5_spawn {
                let (next_func, ctx) = api::spawn(&mut self.context);
                new_ctx = Some(ctx);
                (next_func, true)
            } else if func == terminate {
                self.is_running = false;
                (FunctionObject::new_null(), true)
            } else {
                (func.invoke(&mut self.context), false)
            };
            self.context.update_current_func(next_func);
            if suspends {
                break;
            }
        }
        new_ctx
    }
}

#[derive(Clone, Debug)]
#[repr(C)]
struct ObjectPtr(*mut ());

impl ObjectPtr {
    fn as_ref<T>(&self) -> &T {
        unsafe { &*(self.0 as *const T) }
    }

    fn as_mut<T>(&mut self) -> &mut T {
        unsafe { &mut *(self.0 as *mut T) }
    }
}
extern "C" {
    fn runtime_info_get_entry_point() -> UserFunction;
}

#[no_mangle]
pub extern "C" fn gox5_append(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::append(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_concat(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::concat(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_func_for_pc(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::func_for_pc(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_func_name(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::func_name(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_chan(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::make_chan(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_closure(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::make_closure(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_string_from_byte_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    api::make_string_from_byte_slice(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_string_from_rune(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::make_string_from_rune(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_string_from_rune_slice(
    ctx: &mut LightWeightThreadContext,
) -> FunctionObject {
    api::make_string_from_rune_slice(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::new(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_recv(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_schedule(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_send(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_spawn(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_split(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::split(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_strview(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::strview(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_value_of(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::value_of(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_value_pointer(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::value_pointer(ctx)
}

extern "C" fn terminate(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

pub trait ObjectAllocator {
    fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut ();
    fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut ();
}

struct RuntimeObjectAllocator;

impl RuntimeObjectAllocator {
    fn new() -> Self {
        RuntimeObjectAllocator
    }
}

impl ObjectAllocator for RuntimeObjectAllocator {
    fn allocate(&mut self, size: usize, _destructor: fn(*mut ())) -> *mut () {
        let alignment = mem::size_of::<isize>();
        let size = (size + alignment - 1) / alignment * alignment;
        let buf: Vec<isize> = Vec::with_capacity(size);
        let ptr = buf.leak().as_mut_ptr();
        ptr as *mut ()
    }

    fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
        unsafe {
            let stack_area_addr = libc::mmap(
                ptr::null_mut(),
                4096 * (num_pages + 1),
                libc::PROT_NONE,
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
            stack_start_addr as *mut ()
        }
    }
}

fn create_light_weight_thread_context(
    global_context: GlobalContextPtr,
) -> LightWeightThreadContext {
    let stack_start_addr = global_context
        .process(|mut global_context| global_context.allocator().allocate_guarded_pages(1));
    let prev_func = UserFunction::new(terminate);
    LightWeightThreadContext::new(
        global_context,
        stack_start_addr as *mut StackFrame,
        prev_func,
    )
}

#[cfg_attr(not(test), no_mangle)]
fn main() {
    let allocator = Box::new(RuntimeObjectAllocator::new());
    let global_context = global_context::create_global_context(allocator);
    let mut ctx = create_light_weight_thread_context(global_context);
    let entry_func = unsafe { runtime_info_get_entry_point() };
    let entry_func = FunctionObject::from_user_function(entry_func);
    ctx.set_up(
        entry_func,
        mem::size_of::<isize>(),
        ObjectPtr(ptr::NonNull::dangling().as_ptr()),
        0,
    );

    let mut run_queue = VecDeque::new();
    let mut main_light_weight_thread = LightWeightThread::new(ctx);
    main_light_weight_thread.set_main();
    main_light_weight_thread.start();
    run_queue.push_back(main_light_weight_thread);
    while let Some(mut light_weight_thread) = run_queue.pop_front() {
        let new_ctx = light_weight_thread.execute();
        if let Some(new_ctx) = new_ctx {
            let mut new_light_weight_thread = LightWeightThread::new(new_ctx);
            new_light_weight_thread.start();
            run_queue.push_back(new_light_weight_thread);
        }
        if light_weight_thread.is_running() {
            run_queue.push_back(light_weight_thread);
        } else if light_weight_thread.is_main() {
            break;
        }
    }

    process::exit(0);
}

#[cfg(test)]
mod tests {
    use crate::api::allocate_channel;

    use super::*;

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
            let size = (size + alignment - 1) / alignment * alignment;
            let buf: Vec<isize> = Vec::with_capacity(size);
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, num_pages: usize) -> *mut () {
            let size = num_pages * 4096;
            self.allocate(size, |_| {})
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for allocated_object in &self.allocated_objects {
                (allocated_object.destructor)(allocated_object.ptr);
                unsafe {
                    Vec::from_raw_parts(
                        allocated_object.ptr as *mut isize,
                        0,
                        allocated_object.size,
                    );
                }
            }
        }
    }

    #[test]
    fn test_create_light_weight_thread_context() {
        let allocator = Box::new(MockObjectAllocator::new());
        let global_context = global_context::create_global_context(allocator);
        let ctx = create_light_weight_thread_context(global_context.dupulicate());
        assert_eq!(ctx.global_context, global_context);
        assert!(ctx.prev_func == terminate);
    }

    #[test]
    fn test_allocate_channel() {
        let allocator = Box::new(MockObjectAllocator::new());
        let global_context = global_context::create_global_context(allocator);
        let mut ctx = create_light_weight_thread_context(global_context.dupulicate());
        let channel = allocate_channel(&mut ctx, 1);
        assert_ne!(channel, ptr::null_mut());
    }

    unsafe extern "C" fn user_function(ctx: &mut LightWeightThreadContext) -> FunctionObject {
        ctx.marker = 0x12345678;
        FunctionObject::new_null()
    }

    #[test]
    fn test_invoke_user_function() {
        let allocator = Box::new(MockObjectAllocator::new());
        let global_context = global_context::create_global_context(allocator);
        let mut ctx = create_light_weight_thread_context(global_context.dupulicate());
        let func = UserFunction::new(user_function);
        func.invoke(&mut ctx);
        assert_eq!(ctx.marker, 0x12345678);
    }
}
