mod api;
mod defer_stack;
mod global_context;
mod light_weight_thread;
mod object;
mod type_id;

use std::ffi;
use std::mem;
use std::process;
use std::ptr;

use defer_stack::DeferStack;
use global_context::GlobalContextPtr;
use light_weight_thread::LightWeightThreadContext;
use object::interface::Interface;
use object::string::StringObject;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct FunctionObject(*const ());

#[repr(C)]
struct ClosureLayout {
    func: UserFunction,
    object_ptrs: Vec<ObjectPtr>, // ToDo: fix not to use Vec
}

impl ClosureLayout {
    fn new(func: UserFunction, object_ptrs: Vec<ObjectPtr>) -> Self {
        Self { func, object_ptrs }
    }
}

impl FunctionObject {
    pub fn from_user_function(user_function: UserFunction) -> Self {
        FunctionObject(user_function.0 as *const ())
    }

    pub fn from_closure_layout_ptr(closure_layout_ptr: *const ()) -> Self {
        let addr = closure_layout_ptr as usize;
        let flag = 1 << 63;
        assert_eq!(addr & flag, 0);
        FunctionObject((addr | flag) as *const ())
    }

    pub fn new_null() -> Self {
        FunctionObject(ptr::null_mut())
    }

    pub fn extract_user_function(&self) -> (UserFunction, Option<*mut ()>) {
        let addr = self.0 as usize;
        let flag = 1 << 63;
        if (addr & flag) == 0 {
            let func = unsafe { mem::transmute::<*const (), UserFunction>(self.0) };
            return (func, None);
        }
        let ptr = (addr & !flag) as *mut () as *mut ClosureLayout;
        unsafe {
            let closure_layout = &mut *ptr;
            let func = closure_layout.func.clone();
            let object_ptrs = closure_layout.object_ptrs.as_mut_ptr() as *mut ();
            (func, Some(object_ptrs))
        }
    }
}

type UserFunctionInner = unsafe extern "C" fn(&mut LightWeightThreadContext) -> FunctionObject;

#[derive(Clone)]
#[repr(C)]
pub struct UserFunction(UserFunctionInner);

impl UserFunction {
    pub fn new(user_function: UserFunctionInner) -> Self {
        UserFunction(user_function)
    }

    pub fn invoke(&self, ctx: &mut LightWeightThreadContext) -> FunctionObject {
        unsafe { self.0(ctx) }
    }
}

impl PartialEq<UserFunctionInner> for UserFunction {
    fn eq(&self, other: &UserFunctionInner) -> bool {
        let lhs = self.0 as *const ();
        let rhs = *other as *const ();
        lhs == rhs
    }
}

#[repr(C)]
pub struct StackFrame {
    common: StackFrameCommon,
    additional_words: [*const (); 0],
}

#[repr(C)]
struct StackFrameCommon {
    resume_func: FunctionObject,
    prev_stack_pointer: *mut StackFrame,
    free_vars: *mut (),
    defer_stack: DeferStack,
}

impl StackFrameCommon {
    fn prev_stack_frame_mut<T>(&mut self) -> &mut T {
        let p = self.prev_stack_pointer as *mut T;
        unsafe { &mut *p }
    }

    fn defer_stack_mut(&mut self) -> &mut DeferStack {
        &mut self.defer_stack
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

    fn is_null(&self) -> bool {
        self.0.is_null()
    }
}
extern "C" {
    fn runtime_info_get_entry_point() -> UserFunction;
    fn runtime_info_get_init_point() -> UserFunction;
}

#[no_mangle]
pub extern "C" fn gox5_search_method(
    interface: *const (),
    method_name: StringObject,
) -> FunctionObject {
    let interface = unsafe { &*(interface as *const Interface) };
    let method = interface.search(method_name);
    method.unwrap_or_else(FunctionObject::new_null)
}

extern "C" fn terminate(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.terminate();
    if ctx.is_main() {
        process::exit(0);
    }
    FunctionObject::from_user_function(UserFunction::new(terminate))
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
        let buf: Vec<isize> = vec![0; size];
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
    entry_func: FunctionObject,
) -> LightWeightThreadContext {
    let (id, stack_start_addr) = global_context.process(|mut global_context| {
        let id = global_context.issue_light_weight_thread_id();
        let addr = global_context.allocator().allocate_guarded_pages(10);
        (id, addr)
    });
    let prev_func = UserFunction::new(terminate);
    LightWeightThreadContext::new(
        id,
        global_context,
        stack_start_addr as *mut StackFrame,
        entry_func,
        prev_func,
    )
}

extern "C" fn enter_main(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.call::<StackFrameCommon>(
        0,
        &[],
        FunctionObject::from_user_function(UserFunction::new(terminate)),
    );
    FunctionObject::from_user_function(unsafe { runtime_info_get_entry_point() })
}

fn execute(ctx: &mut LightWeightThreadContext) {
    assert!(!ctx.is_terminated());
    ctx.resume();
    while !ctx.is_suspended() {
        let func = ctx.prepare_user_function();
        let next_func = func.invoke(ctx);
        ctx.update_current_func(next_func);
    }
}

#[cfg_attr(not(test), no_mangle)]
fn main() {
    let allocator = Box::new(RuntimeObjectAllocator::new());
    let global_context = global_context::create_global_context(allocator);

    let init_func = unsafe { runtime_info_get_init_point() };
    let init_func = FunctionObject::from_user_function(init_func);
    let mut ctx = create_light_weight_thread_context(global_context.dupulicate(), init_func);
    ctx.call::<StackFrameCommon>(
        mem::size_of::<isize>(),
        &[],
        FunctionObject::from_user_function(UserFunction::new(enter_main)),
    );

    global_context.process(|mut global_context| {
        global_context.push_light_weight_thread(ctx);
    });

    while let Some(mut ctx) =
        global_context.process(|mut global_context| global_context.pop_light_weight_thread())
    {
        execute(&mut ctx);
        if !ctx.is_terminated() {
            global_context.process(|mut global_context| {
                global_context.push_light_weight_thread(ctx);
            });
        }
    }

    unreachable!()
}

#[cfg(test)]
mod tests {
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
        let func = FunctionObject::from_user_function(UserFunction::new(user_function));
        let ctx = create_light_weight_thread_context(global_context.dupulicate(), func);
        assert_eq!(ctx.id(), 0);
        assert!(ctx.is_main());
        assert_eq!(ctx.global_context(), &global_context);
        assert!(ctx.is_stack_empty());
        assert!(!ctx.is_suspended());
        assert!(!ctx.is_terminated());
        assert!(!ctx.is_panicking());
    }

    unsafe extern "C" fn user_function(ctx: &mut LightWeightThreadContext) -> FunctionObject {
        ctx.terminate(); // observable side effect
        FunctionObject::new_null()
    }

    #[test]
    fn test_invoke_user_function() {
        let allocator = Box::new(MockObjectAllocator::new());
        let global_context = global_context::create_global_context(allocator);
        let mut ctx = create_light_weight_thread_context(
            global_context.dupulicate(),
            FunctionObject::new_null(),
        );
        assert!(!ctx.is_terminated());
        let func = UserFunction::new(user_function);
        let result = func.invoke(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert!(ctx.is_terminated());
    }
}
