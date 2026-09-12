mod allocator;
mod api;
mod defer_stack;
mod global_context;
mod light_weight_thread;
mod object;
mod type_id;
mod word_chunk;

use std::mem;
use std::process;
use std::ptr;

use allocator::{ObjectAllocator, ObjectAllocatorPtr};
use defer_stack::DeferStack;
use global_context::GlobalContextPtr;
use light_weight_thread::LightWeightThreadContext;

#[derive(Clone, PartialEq, Eq, Debug)]
#[repr(C)]
pub struct FunctionObject(*const ());

#[repr(C)]
struct ClosureLayout {
    func: UserFunction,
    object_ptrs: word_chunk::WordChunk,
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
        let closure_layout = unsafe { &mut *ptr };
        let func = closure_layout.func.clone();
        let wc_ptr = ptr::addr_of!(closure_layout.object_ptrs) as *const u8;
        let object_ptrs = unsafe { wc_ptr.add(mem::size_of::<usize>()) as *mut () };
        (func, Some(object_ptrs))
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
unsafe extern "C" {
    fn runtime_info_get_entry_point() -> UserFunction;
    fn runtime_info_get_init_point() -> UserFunction;
}

extern "C" fn terminate(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    ctx.suspend();
    ctx.terminate();
    if ctx.is_main() {
        process::exit(0);
    }
    FunctionObject::from_user_function(UserFunction::new(terminate))
}

fn create_light_weight_thread_context(
    global_context: GlobalContextPtr,
    entry_func: FunctionObject,
) -> LightWeightThreadContext {
    let (id, stack_start_addr) = global_context.process(|mut global_context| {
        let id = global_context.issue_light_weight_thread_id();
        let addr = global_context.allocator().allocate_guarded_pages(200);
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
    let prev_stack_pointer = ctx.stack_pointer();
    ctx.push_frame(
        prev_stack_pointer,
        None,
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

#[cfg_attr(not(test), unsafe(no_mangle))]
fn main() {
    let allocator = ObjectAllocator::new();
    let global_context = global_context::create_global_context(allocator);

    let init_func = unsafe { runtime_info_get_init_point() };
    let init_func = FunctionObject::from_user_function(init_func);
    let mut ctx = create_light_weight_thread_context(global_context.dupulicate(), init_func);
    let prev_stack_pointer = ctx.stack_pointer();
    ctx.grow_stack(mem::size_of::<isize>());
    ctx.push_frame(
        prev_stack_pointer,
        Some(prev_stack_pointer as *const ()),
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
                if let Some(slot) = ctx.take_coro_slot() {
                    global_context.park_coro(slot, ctx);
                } else {
                    global_context.push_light_weight_thread(ctx);
                }
            });
        }
    }

    unreachable!()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_light_weight_thread_context() {
        let global_context = global_context::create_global_context(ObjectAllocator::new());
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
        let global_context = global_context::create_global_context(ObjectAllocator::new());
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
