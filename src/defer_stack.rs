use std::ptr;

use crate::FunctionObject;
use crate::word_chunk::WordChunk;

#[repr(C)]
pub(crate) struct DeferStackEntry {
    next: Option<ptr::NonNull<DeferStackEntry>>,
    func: FunctionObject,
    result_size: usize,
    args: ptr::NonNull<WordChunk>,
}

impl DeferStackEntry {
    pub(crate) fn new(
        func: FunctionObject,
        result_size: usize,
        args: ptr::NonNull<WordChunk>,
    ) -> Self {
        Self {
            next: None,
            func,
            result_size,
            args,
        }
    }

    pub(crate) fn func(&self) -> FunctionObject {
        self.func.clone()
    }

    pub(crate) fn result_size(&self) -> usize {
        self.result_size
    }

    pub(crate) fn args(&self) -> &[*const ()] {
        unsafe { WordChunk::as_slice_raw(self.args.as_ptr()) }
    }
}

#[repr(C)]
pub(crate) struct DeferStack(Option<ptr::NonNull<DeferStackEntry>>);

impl DeferStack {
    pub(crate) fn new() -> Self {
        Self(None)
    }

    pub(crate) fn push(&mut self, mut entry: ptr::NonNull<DeferStackEntry>) {
        unsafe { entry.as_mut() }.next = self.0.take();
        self.0 = Some(entry);
    }

    pub(crate) fn pop(&mut self) -> Option<ptr::NonNull<DeferStackEntry>> {
        let mut entry = self.0.take()?;
        self.0 = unsafe { entry.as_mut() }.next.take();
        Some(entry)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_defer_stack_new_is_empty() {
        let mut stack = DeferStack::new();
        assert!(stack.pop().is_none());
    }

    #[test]
    fn test_defer_stack_push_pop_single() {
        let mut stack = DeferStack::new();
        let entry = Box::into_raw(Box::new(DeferStackEntry::new(
            FunctionObject::new_null(),
            0,
            ptr::NonNull::dangling(),
        )));
        let nn = unsafe { ptr::NonNull::new_unchecked(entry) };
        stack.push(nn);
        let popped = stack.pop().unwrap();
        assert_eq!(popped.as_ptr(), entry);
        unsafe {
            drop(Box::from_raw(entry));
        }
    }

    #[test]
    fn test_defer_stack_lifo_order() {
        let mut stack = DeferStack::new();
        let entries: Vec<*mut DeferStackEntry> = (0..3)
            .map(|_| {
                Box::into_raw(Box::new(DeferStackEntry::new(
                    FunctionObject::new_null(),
                    0,
                    ptr::NonNull::dangling(),
                )))
            })
            .collect();
        for e in &entries {
            stack.push(unsafe { ptr::NonNull::new_unchecked(*e) });
        }
        for e in entries.iter().rev() {
            let popped = stack.pop().unwrap();
            assert_eq!(popped.as_ptr(), *e);
        }
        assert!(stack.pop().is_none());
        for e in entries {
            unsafe {
                drop(Box::from_raw(e));
            }
        }
    }

    #[test]
    fn test_defer_stack_pop_empty_returns_none() {
        let mut stack = DeferStack::new();
        assert!(stack.pop().is_none());
        assert!(stack.pop().is_none());
    }

    #[test]
    fn test_defer_stack_push_pop_interleave() {
        let mut stack = DeferStack::new();
        let e1 = Box::into_raw(Box::new(DeferStackEntry::new(
            FunctionObject::new_null(),
            0,
            ptr::NonNull::dangling(),
        )));
        let e2 = Box::into_raw(Box::new(DeferStackEntry::new(
            FunctionObject::new_null(),
            0,
            ptr::NonNull::dangling(),
        )));
        let nn1 = unsafe { ptr::NonNull::new_unchecked(e1) };
        let nn2 = unsafe { ptr::NonNull::new_unchecked(e2) };
        stack.push(nn1);
        let popped = stack.pop().unwrap();
        assert_eq!(popped.as_ptr(), e1);
        stack.push(nn2);
        let popped = stack.pop().unwrap();
        assert_eq!(popped.as_ptr(), e2);
        assert!(stack.pop().is_none());
        unsafe {
            drop(Box::from_raw(e1));
            drop(Box::from_raw(e2));
        }
    }

    #[test]
    fn test_defer_stack_entry_accessors() {
        let entry = DeferStackEntry::new(FunctionObject::new_null(), 42, ptr::NonNull::dangling());
        assert_eq!(entry.func(), FunctionObject::new_null());
        assert_eq!(entry.result_size(), 42);
    }
}
