mod api;
mod channel;
mod global_context;

use std::ffi;
use std::mem;
use std::ptr;
use std::slice;

use async_recursion::async_recursion;

use api::{NextUserFunctionType, StackFrame, UserFunctionType};
use global_context::GlobalContextPtr;

#[repr(C)]
pub struct LightWeightThreadContext<'a> {
    global_context: GlobalContextPtr,
    stack_pointer: &'a mut StackFrame,
    prev_func: UserFunctionType,
    marker: isize,
}

unsafe impl<'a> Send for LightWeightThreadContext<'a> {}

#[derive(Clone, Debug)]
#[repr(C)]
struct ObjectPtr(*mut ());

impl ObjectPtr {
    fn as_ref<T>(&self) -> &T {
        unsafe { &*(self.0 as *const T) }
    }
}
unsafe impl Send for ObjectPtr {}

extern "C" {
    fn test_entry_point_num() -> libc::size_t;
    fn test_entry_point_name(i: libc::size_t) -> *const libc::c_char;
    fn test_entry_point_function(i: libc::size_t) -> UserFunctionType;
}

#[no_mangle]
pub extern "C" fn gox5_append(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    api::append(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_chan(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    api::make_chan(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_new(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    api::new(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_recv(_ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_send(_ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_spawn(_ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unreachable!()
}

extern "C" fn terminate(_ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
    unreachable!()
}

#[async_recursion]
pub async fn spawn_wrapper(ctx: &mut LightWeightThreadContext<'_>) -> NextUserFunctionType {
    api::spawn(ctx).await
}

async fn start_light_weight_thread<'a>(
    entry_func: UserFunctionType,
    ctx: &mut LightWeightThreadContext<'a>,
    result_size: usize,
    arg_buffer_ptr: ObjectPtr,
    num_arg_buffer_words: usize,
) {
    unsafe {
        let result_pointer = ctx.stack_pointer as *mut StackFrame as *mut ();
        let stack_pointer = (result_pointer as *mut u8).add(result_size);
        ctx.stack_pointer = &mut *(stack_pointer as *mut StackFrame);
        let words = slice::from_raw_parts_mut(ctx.stack_pointer.words.as_mut_ptr(), 4);
        words[0] = terminate as *mut ();
        words[1] = ptr::null_mut();
        let mut arg_base = 2;
        if result_size > 0 {
            words[arg_base] = result_pointer;
            arg_base += 1;
        }
        let src_arg_buffer_ptr = arg_buffer_ptr.as_ref::<usize>();
        let dst_arg_buffer_ptr = &mut words[arg_base] as *mut *mut () as *mut usize;
        ptr::copy_nonoverlapping(src_arg_buffer_ptr, dst_arg_buffer_ptr, num_arg_buffer_words);
    }

    let mut func = entry_func;
    loop {
        let next_func = if func == gox5_send {
            api::send(ctx).await
        } else if func == gox5_recv {
            api::recv(ctx).await
        } else if func == gox5_spawn {
            spawn_wrapper(ctx).await
        } else if func == terminate {
            break;
        } else {
            func.invoke(ctx)
        };
        ctx.prev_func = func;
        func = unsafe { mem::transmute::<NextUserFunctionType, UserFunctionType>(next_func) }
    }
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

fn create_light_weight_thread_context<'a>(
    global_context: GlobalContextPtr,
) -> LightWeightThreadContext<'a> {
    let stack_start_addr = global_context
        .process(|mut global_context| global_context.allocator().allocate_guarded_pages(1));
    let prev_func = UserFunctionType::new(terminate);
    LightWeightThreadContext {
        global_context,
        stack_pointer: unsafe { &mut *(stack_start_addr as *mut StackFrame) },
        prev_func,
        marker: 0xdeadbeef,
    }
}

#[cfg_attr(not(test), no_mangle)]
#[tokio::main]
async fn main() {
    let allocator = Box::new(RuntimeObjectAllocator::new());
    let global_context = global_context::create_global_context(allocator);
    let mut ctx = create_light_weight_thread_context(global_context);
    let stack_pointer = ctx.stack_pointer as *mut StackFrame;
    unsafe {
        for i in 0..test_entry_point_num() {
            let entry_func = test_entry_point_function(i);
            start_light_weight_thread(
                entry_func,
                &mut ctx,
                mem::size_of::<isize>(),
                ObjectPtr(ptr::NonNull::dangling().as_ptr()),
                0,
            )
            .await;
            let result = *(stack_pointer as *const isize);
            let name = ffi::CStr::from_ptr(test_entry_point_name(i))
                .to_str()
                .unwrap();
            println!("{}: {}", name, result);

            ctx.stack_pointer = &mut *stack_pointer;
        }
    }
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

    unsafe extern "C" fn user_function(ctx: &mut LightWeightThreadContext) -> NextUserFunctionType {
        ctx.marker = 0x12345678;
        NextUserFunctionType::new_null()
    }

    #[test]
    fn test_invoke_user_function() {
        let allocator = Box::new(MockObjectAllocator::new());
        let global_context = global_context::create_global_context(allocator);
        let mut ctx = create_light_weight_thread_context(global_context.dupulicate());
        let func = UserFunctionType::new(user_function);
        func.invoke(&mut ctx);
        assert_eq!(ctx.marker, 0x12345678);
    }
}
