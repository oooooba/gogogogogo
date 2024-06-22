use std::ptr;

use crate::word_chunk::WordChunk;
use crate::FunctionObject;

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
        unsafe { self.args.as_ref().as_slice() }
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
