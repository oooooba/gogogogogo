mod api;
mod defer_stack;
mod global_context;
mod interface;
mod object;
mod type_id;

use std::ffi;
use std::mem;
use std::process;
use std::ptr;
use std::slice;

use defer_stack::DeferStack;
use global_context::GlobalContextPtr;
use interface::Interface;
use object::string::StringObject;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct FunctionObject(*const ());

#[repr(C)]
struct ClosureLayout {
    func: UserFunction,
    object_ptrs: Vec<ObjectPtr>,
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
    fn new(
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

    fn call<T>(&mut self, result_size: usize, args: &[*const ()], resume_func: FunctionObject) {
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

    fn prepare_user_function(&mut self) -> UserFunction {
        let (func, object_ptrs) = self.current_func.extract_user_function();
        if let Some(object_ptrs) = object_ptrs {
            unsafe {
                let words = slice::from_raw_parts_mut(self.stack_pointer as *mut *mut (), 3);
                words[2] = object_ptrs; // free_vars
            }
        }
        func
    }

    fn id(&self) -> usize {
        self.id
    }

    fn global_context(&self) -> &GlobalContextPtr {
        &self.global_context
    }

    fn update_current_func(&mut self, func: FunctionObject) {
        self.prev_func = self.current_func.extract_user_function().0;
        self.current_func = func
    }

    fn stack_frame<T>(&self) -> &T {
        let p = self.stack_pointer as *const T;
        unsafe { &*p }
    }

    fn stack_frame_mut<T>(&mut self) -> &mut T {
        let p = self.stack_pointer as *mut T;
        unsafe { &mut *p }
    }

    pub fn is_stack_empty(&self) -> bool {
        assert!(self.initial_stack_pointer <= self.stack_pointer);
        self.initial_stack_pointer == self.stack_pointer
    }

    fn leave(&mut self) -> FunctionObject {
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

    fn is_main(&self) -> bool {
        self.id == 0
    }

    fn is_suspended(&self) -> bool {
        self.control_flags & 0b1 > 0
    }

    fn suspend(&mut self) {
        self.control_flags |= 0b1;
    }

    fn resume(&mut self) {
        self.control_flags &= !0b1;
    }

    fn is_terminated(&self) -> bool {
        self.control_flags & 0b10 > 0
    }

    fn terminate(&mut self) {
        self.control_flags |= 0b10;
    }

    fn enter_panic(&mut self, data: Interface) {
        self.control_flags |= 0b100;
        self.panic_data = data;
    }

    fn exit_panic(&mut self) -> Interface {
        assert!(self.is_panicking());
        self.control_flags &= !0b100;
        self.panic_data.clone()
    }

    fn is_panicking(&self) -> bool {
        self.control_flags & 0b100 > 0
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
pub extern "C" fn gox5_make_closure(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::make_closure(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_make_interface(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::make_interface(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_new(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    api::new(ctx)
}

#[no_mangle]
pub extern "C" fn gox5_schedule(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
}

#[no_mangle]
pub extern "C" fn gox5_spawn(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
    unreachable!()
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
        let next_func = if func == gox5_schedule {
            api::schedule(ctx)
        } else if func == gox5_spawn {
            api::spawn(ctx)
        } else {
            func.invoke(ctx)
        };
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
        assert_eq!(ctx.global_context, global_context);
        assert!(
            ctx.current_func
                == FunctionObject::from_user_function(UserFunction::new(user_function))
        );
        assert!(ctx.prev_func == terminate);
    }

    unsafe extern "C" fn user_function(ctx: &mut LightWeightThreadContext) -> FunctionObject {
        ctx.marker = 0x12345678;
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
        let func = UserFunction::new(user_function);
        func.invoke(&mut ctx);
        assert_eq!(ctx.marker, 0x12345678);
    }
}
