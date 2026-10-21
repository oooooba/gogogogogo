use std::cell::Cell;
use std::ffi;
use std::ptr;

/// The granularity of the pages the pager hands out and takes back.
pub(crate) const PAGE_SIZE: usize = 4096;

/// The number of guard pages a region carries, one below it and one above it.
#[cfg(not(miri))]
const GUARD_PAGES: usize = 2;

/// Allocates and deallocates pages of `PAGE_SIZE` bytes. A region of pages has
/// a guard page above and below it that stays unmapped, so that running off
/// either end of a region (a goroutine stack overflowing its pages, for
/// instance) traps instead of silently writing into whatever the kernel mapped
/// next.
///
/// There is one pager: `main` creates it and shares it through an `Rc` with the
/// global context, which hands it to the systems that need pages (the object
/// allocator maps its large allocations from them, the scheduler maps goroutine
/// stacks from them).
pub(crate) struct Pager {
    /// The pages handed out and not given back yet. The pager is shared
    /// through an `Rc` and the runtime runs every goroutine on one thread, so a
    /// `Cell` is all it takes to keep the count.
    allocated_pages: Cell<usize>,
}

impl Pager {
    pub(crate) fn new() -> Self {
        Pager {
            allocated_pages: Cell::new(0),
        }
    }

    /// Maps `num_pages` writable pages between two guard pages and returns the
    /// address of the first page that can be used.
    pub(crate) fn allocate(&self, num_pages: usize, align: Option<usize>) -> *mut () {
        #[cfg(miri)]
        let region = {
            let align = align.unwrap_or(PAGE_SIZE).max(PAGE_SIZE);
            let total_pages = num_pages + align / PAGE_SIZE;
            unsafe {
                let base_addr = libc::mmap(
                    ptr::null_mut(),
                    PAGE_SIZE * total_pages,
                    libc::PROT_READ | libc::PROT_WRITE,
                    libc::MAP_ANONYMOUS | libc::MAP_PRIVATE,
                    -1,
                    0,
                );
                if base_addr == libc::MAP_FAILED {
                    let message = ffi::CString::new("allocate region").unwrap();
                    libc::perror(message.as_ptr());
                    panic!();
                }
                let usable_start =
                    base_addr.with_addr((base_addr.addr() + PAGE_SIZE + align - 1) & !(align - 1));
                let metadata = usable_start.cast::<u8>().sub(PAGE_SIZE);
                metadata.cast::<*mut libc::c_void>().write(base_addr);
                metadata.cast::<usize>().add(1).write(total_pages);
                usable_start
            }
        };
        #[cfg(not(miri))]
        let region = {
            let (total_pages, align) = match align {
                None => (num_pages + GUARD_PAGES, PAGE_SIZE),
                Some(align) => {
                    assert!(align != 0, "an aligned region needs a nonzero alignment");
                    assert!(
                        align.is_power_of_two(),
                        "an aligned region needs a power-of-two alignment"
                    );
                    assert_eq!(
                        align % PAGE_SIZE,
                        0,
                        "an aligned region needs an alignment that is a multiple of the page size"
                    );
                    (num_pages + GUARD_PAGES + align / PAGE_SIZE - 1, align)
                }
            };
            unsafe {
                let base_addr = libc::mmap(
                    ptr::null_mut(),
                    PAGE_SIZE * total_pages,
                    libc::PROT_NONE,
                    libc::MAP_ANONYMOUS | libc::MAP_PRIVATE,
                    -1,
                    0,
                );
                if base_addr == libc::MAP_FAILED {
                    let message = ffi::CString::new("allocate region").unwrap();
                    libc::perror(message.as_ptr());
                    panic!();
                }
                let usable_start =
                    base_addr.with_addr((base_addr.addr() + PAGE_SIZE + align - 1) & !(align - 1));
                let lower_slack = usable_start.addr() - base_addr.addr() - PAGE_SIZE;
                if lower_slack > 0 {
                    libc::munmap(base_addr, lower_slack);
                }
                let guard_end = usable_start.cast::<u8>().add(PAGE_SIZE * (num_pages + 1));
                let map_end = base_addr.cast::<u8>().add(PAGE_SIZE * total_pages);
                if guard_end.addr() < map_end.addr() {
                    libc::munmap(
                        guard_end.cast::<libc::c_void>(),
                        map_end.addr() - guard_end.addr(),
                    );
                }
                let ret = libc::mprotect(
                    usable_start,
                    PAGE_SIZE * num_pages,
                    libc::PROT_READ | libc::PROT_WRITE,
                );
                if ret != 0 {
                    let message = ffi::CString::new("region protection mode").unwrap();
                    libc::perror(message.as_ptr());
                    panic!();
                }
                usable_start
            }
        };
        self.allocated_pages
            .set(self.allocated_pages.get() + num_pages);
        region as *mut ()
    }

    /// Gives the region of `num_pages` pages returned by `allocate` back to the
    /// system, both guard pages included. Giving back more pages than were
    /// handed out means a region is released twice, which the count refuses.
    pub(crate) fn deallocate(&self, ptr: *mut (), num_pages: usize) {
        let remaining = self
            .allocated_pages
            .get()
            .checked_sub(num_pages)
            .expect("a region of pages was given back twice");
        #[cfg(miri)]
        unsafe {
            let metadata = ptr.cast::<u8>().sub(PAGE_SIZE);
            let base_addr = metadata.cast::<*mut libc::c_void>().read();
            let total_pages = metadata.cast::<usize>().add(1).read();
            libc::munmap(base_addr, PAGE_SIZE * total_pages);
        }
        #[cfg(not(miri))]
        unsafe {
            let base_addr = ptr.cast::<u8>().sub(PAGE_SIZE).cast::<libc::c_void>();
            libc::munmap(base_addr, PAGE_SIZE * (num_pages + GUARD_PAGES));
        }
        self.allocated_pages.set(remaining);
    }

    /// The number of pages handed out and not given back yet.
    #[cfg(test)]
    pub(crate) fn allocated_pages(&self) -> usize {
        self.allocated_pages.get()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_allocate_returns_writable_pages() {
        let pager = Pager::new();
        let ptr = pager.allocate(2, None);
        assert!(!ptr.is_null());
        assert_eq!(ptr as usize % PAGE_SIZE, 0);
        unsafe {
            (ptr as *mut usize).write(1234);
            assert_eq!((ptr as *const usize).read(), 1234);
            // The last page of the region is writable too.
            let last = (ptr as *mut u8).add(PAGE_SIZE) as *mut usize;
            last.write(5678);
            assert_eq!(last.read(), 5678);
        }
        pager.deallocate(ptr, 2);
    }

    /// Whether the region of `num_pages` pages the pager hands out is unmapped
    /// again once it has been given back. Touching the pages would fault, which
    /// a test cannot catch, so a forked child checks with mprotect instead,
    /// which fails on an address that is not mapped anymore.
    ///
    /// The child maps the region and gives it back itself: an address that was
    /// unmapped here is free for another thread to map again, and such a
    /// mapping is what makes the check fail for the wrong reason. The child has
    /// its own address space, in which nothing else runs.
    #[cfg(not(miri))]
    fn deallocated_region_is_unmapped(num_pages: usize) -> bool {
        // SAFETY: the child only maps a region, unmaps it, calls mprotect and
        // exits, all of which are async-signal-safe.
        let pid = unsafe { libc::fork() };
        assert!(pid >= 0, "fork failed");
        if pid == 0 {
            let pager = Pager::new();
            let ptr = pager.allocate(num_pages, None);
            pager.deallocate(ptr, num_pages);
            let ret = unsafe {
                libc::mprotect(
                    ptr as *mut libc::c_void,
                    PAGE_SIZE * num_pages,
                    libc::PROT_READ | libc::PROT_WRITE,
                )
            };
            unsafe { libc::_exit(if ret != 0 { 0 } else { 1 }) };
        }
        let mut status = 0;
        assert_eq!(unsafe { libc::waitpid(pid, &mut status, 0) }, pid);
        libc::WIFEXITED(status) && libc::WEXITSTATUS(status) == 0
    }

    #[test]
    #[cfg(not(miri))]
    fn test_deallocate_unmaps_the_region() {
        assert!(
            deallocated_region_is_unmapped(1),
            "mprotect succeeded on a page that was given back"
        );
    }

    /// Whether writing to `address` in a forked child dies from SIGSEGV. The
    /// write cannot be done in this process, because a guard page fault takes
    /// the whole test binary down with it.
    #[cfg(not(miri))]
    fn write_traps(address: *mut u8) -> bool {
        // SAFETY: the child only writes to `address` and exits, both of which
        // are async-signal-safe.
        let pid = unsafe { libc::fork() };
        assert!(pid >= 0, "fork failed");
        if pid == 0 {
            unsafe {
                address.write_volatile(1);
                libc::_exit(0);
            }
        }
        let mut status = 0;
        assert_eq!(unsafe { libc::waitpid(pid, &mut status, 0) }, pid);
        libc::WIFSIGNALED(status) && libc::WTERMSIG(status) == libc::SIGSEGV
    }

    #[test]
    #[cfg(not(miri))]
    fn test_allocate_guards_the_pages_above_and_below_the_region() {
        let pager = Pager::new();
        let region = pager.allocate(1, None) as usize;
        assert!(
            write_traps((region - 1) as *mut u8),
            "the page below the region is writable"
        );
        assert!(
            write_traps((region + PAGE_SIZE) as *mut u8),
            "the page above the region is writable"
        );
        // The region itself stays writable, so the same write there does not
        // trap.
        assert!(!write_traps(region as *mut u8));
        pager.deallocate(region as *mut (), 1);
    }

    #[test]
    fn test_allocate_counts_the_pages_and_deallocate_gives_them_back() {
        let pager = Pager::new();
        assert_eq!(pager.allocated_pages(), 0);
        let first = pager.allocate(2, None);
        let second = pager.allocate(3, None);
        assert_eq!(pager.allocated_pages(), 5);
        pager.deallocate(first, 2);
        assert_eq!(pager.allocated_pages(), 3);
        pager.deallocate(second, 3);
        assert_eq!(pager.allocated_pages(), 0);
    }

    #[test]
    #[should_panic(expected = "given back twice")]
    fn test_deallocate_refuses_more_pages_than_were_allocated() {
        let pager = Pager::new();
        let ptr = pager.allocate(1, None);
        pager.deallocate(ptr, 2);
    }

    #[test]
    fn test_allocate_hands_out_distinct_regions() {
        let pager = Pager::new();
        let first = pager.allocate(1, None);
        let second = pager.allocate(1, None);
        assert_ne!(first, second);
        pager.deallocate(first, 1);
        pager.deallocate(second, 1);
    }

    #[test]
    fn test_allocate_with_alignment_returns_aligned_writable_pages() {
        let pager = Pager::new();
        let align = 4 * PAGE_SIZE;
        let ptr = pager.allocate(2, Some(align));
        assert!(!ptr.is_null());
        assert_eq!(ptr as usize % align, 0);
        unsafe {
            (ptr as *mut usize).write(1234);
            assert_eq!((ptr as *const usize).read(), 1234);
            let last = (ptr as *mut u8).add(PAGE_SIZE) as *mut usize;
            last.write(5678);
            assert_eq!(last.read(), 5678);
        }
        pager.deallocate(ptr, 2);
    }

    #[test]
    fn test_allocate_with_alignment_counts_the_pages_and_deallocate_gives_them_back() {
        let pager = Pager::new();
        assert_eq!(pager.allocated_pages(), 0);
        let first = pager.allocate(3, Some(8 * PAGE_SIZE));
        let second = pager.allocate(1, Some(PAGE_SIZE));
        assert_eq!(pager.allocated_pages(), 4);
        pager.deallocate(first, 3);
        assert_eq!(pager.allocated_pages(), 1);
        pager.deallocate(second, 1);
        assert_eq!(pager.allocated_pages(), 0);
    }

    #[test]
    fn test_allocate_with_alignment_hands_out_distinct_regions() {
        let pager = Pager::new();
        let first = pager.allocate(1, Some(PAGE_SIZE));
        let second = pager.allocate(1, Some(PAGE_SIZE));
        assert_ne!(first, second);
        pager.deallocate(first, 1);
        pager.deallocate(second, 1);
    }

    #[test]
    #[should_panic(expected = "given back twice")]
    fn test_deallocate_refuses_an_aligned_region_twice() {
        let pager = Pager::new();
        let ptr = pager.allocate(1, Some(PAGE_SIZE));
        pager.deallocate(ptr, 1);
        pager.deallocate(ptr, 1);
    }

    #[test]
    #[cfg(not(miri))]
    fn test_allocate_with_alignment_guards_the_region() {
        let pager = Pager::new();
        let region = pager.allocate(1, Some(4 * PAGE_SIZE)) as usize;
        assert!(
            write_traps((region - 1) as *mut u8),
            "the page below the aligned region is writable"
        );
        assert!(
            write_traps((region + PAGE_SIZE) as *mut u8),
            "the page above the aligned region is writable"
        );
        assert!(!write_traps(region as *mut u8));
        pager.deallocate(region as *mut (), 1);
    }
}
