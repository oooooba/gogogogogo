use std::collections::{BTreeMap, BTreeSet};
use std::ffi;
use std::mem;
use std::ptr;
use std::ptr::NonNull;
use std::rc::Rc;

use allocator_api2::alloc::{AllocError, Allocator, Layout};

use crate::ClosureLayout;
use crate::FUNCTION_OBJECT_CLOSURE_FLAG;
use crate::light_weight_thread::LightWeightThreadContext;
use crate::object::channel::ChannelObject;
use crate::object::interface::Interface;
use crate::object::map::MapObject;
use crate::pager::PAGE_SIZE;
use crate::pager::Pager;
use crate::type_id::GC_SLOT_DEFER_STACK;
use crate::type_id::GC_SLOT_FREE_VARS;
use crate::type_id::GC_SLOT_FUNCTION;
use crate::type_id::GC_SLOT_INTERFACE;
use crate::type_id::GC_SLOT_POINTER;
use crate::type_id::GC_SLOT_RAW;
use crate::type_id::GC_SLOT_SLICE;
use crate::type_id::GC_SLOT_STRING;
use crate::type_id::TypeId;
use crate::type_id::TypeKind;
use crate::type_id::TypeOffsetVisitor;

pub(crate) struct ObjectAllocator(ObjectAllocatorPtr);

impl ObjectAllocator {
    pub(crate) fn new(pager: Rc<Pager>) -> Self {
        ObjectAllocator(ObjectAllocatorPtr(Box::into_raw(Box::new(
            ObjectAllocatorInner::new(pager),
        ))))
    }

    pub(crate) fn ptr(&self) -> ObjectAllocatorPtr {
        self.0.clone()
    }
}

impl Drop for ObjectAllocator {
    fn drop(&mut self) {
        unsafe {
            Box::from_raw(self.0.0).free_all_allocated_objects();
        }
    }
}

struct ObjectAllocatorInner {
    pager: Rc<Pager>,
    allocated_objects: BTreeMap<usize, AllocatedObject>,
    /// Closure allocations live apart from ordinary objects: a closure is
    /// referenced through a `FunctionObject` whose address has the most
    /// significant bit set, so the GC must distinguish tagged words from plain
    /// pointers. Membership in this map is what makes that call, and the
    /// closure object's own capture descriptor (stored in ClosureLayout)
    /// tells the collector how to trace the captured variables.
    allocated_closures: BTreeMap<usize, AllocatedObject>,
    /// Channel objects self-describe through their element type, which is
    /// registered here when the channel is created.
    allocated_channels: BTreeMap<usize, TypeId>,
    /// Map objects are ordinary allocations whose entries are boxed with the
    /// map's key/value types; membership makes the collector dispatch to the
    /// entry scan instead of the plain-object conservative scan.
    allocated_maps: BTreeSet<usize>,
    global_spans: Vec<GlobalSpan>,
    total_size: usize,
}

#[derive(Clone, Copy)]
struct Span {
    ptr: *mut (),
    size: usize,
}

/// A root that is not an object on the heap: usually a package-level Go
/// variable. It is scanned as a value of `type_id`, or conservatively over
/// its span when the type is unknown (TypeId 0).
#[derive(Clone, Copy)]
struct GlobalSpan {
    ptr: *mut (),
    size: usize,
    type_id: TypeId,
}

struct AllocatedObject {
    span: Span,
    kind: AllocationKind,
    marked: bool,
}

#[derive(Clone, Copy, PartialEq, Debug)]
enum AllocationKind {
    Heap,
    Pages,
}

/// What `mark_object` has to do with an object that is not a plain heap
/// allocation when it is first reached: closures, channels and maps carry
/// their own trace description.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum SpecialObject {
    None,
    Closure,
    Channel,
    Map,
}

const ALLOCATION_ALIGNMENT: usize = mem::size_of::<u128>();
/// A slice header is three words (data, len, cap) in the C and Rust layouts.
const SLICE_HEADER_SIZE: usize = 3 * mem::size_of::<usize>();

/// A string value is two words (data pointer, length).
const STRING_SIZE: usize = 2 * mem::size_of::<usize>();
pub(crate) const MAX_TOTAL_ALLOCATED_SIZE: usize = 1 << 20;
pub(crate) const LARGE_ALLOCATION_THRESHOLD: usize = 128 * 1024;

/// The number of bytes a value of type `typ` occupies, so a collector can
/// check that the storage it is about to read really holds the whole value.
fn value_size(typ: TypeId) -> usize {
    match typ.kind() {
        TypeKind::None => 0,
        TypeKind::String => STRING_SIZE,
        TypeKind::Slice => SLICE_HEADER_SIZE,
        TypeKind::Interface => mem::size_of::<Interface>(),
        _ => typ.size(),
    }
}

impl ObjectAllocatorInner {
    fn new(pager: Rc<Pager>) -> Self {
        ObjectAllocatorInner {
            pager,
            allocated_objects: BTreeMap::new(),
            allocated_closures: BTreeMap::new(),
            allocated_channels: BTreeMap::new(),
            allocated_maps: BTreeSet::new(),
            global_spans: Vec::new(),
            total_size: 0,
        }
    }

    fn allocate(&mut self, size: usize) -> *mut () {
        if size > LARGE_ALLOCATION_THRESHOLD {
            return self.allocate_pages(size);
        }
        Self::allocate_span(&mut self.total_size, size, &mut self.allocated_objects)
    }

    fn allocate_channel(&mut self, size: usize, elem_type: TypeId) -> *mut () {
        let ptr = self.allocate(size);
        if !ptr.is_null() {
            self.allocated_channels.insert(ptr as usize, elem_type);
        }
        ptr
    }

    fn allocate_closure(&mut self, size: usize) -> *mut () {
        Self::allocate_span(&mut self.total_size, size, &mut self.allocated_closures)
    }

    fn allocate_map(&mut self, size: usize) -> *mut () {
        let ptr = self.allocate(size);
        if !ptr.is_null() {
            self.allocated_maps.insert(ptr as usize);
        }
        ptr
    }

    /// Allocates a zero-initialized, 16-byte-aligned heap span and records it
    /// in `destinations` (either the ordinary objects or the closures map).
    fn allocate_span(
        total_size: &mut usize,
        size: usize,
        destinations: &mut BTreeMap<usize, AllocatedObject>,
    ) -> *mut () {
        let size = size.div_ceil(ALLOCATION_ALIGNMENT) * ALLOCATION_ALIGNMENT;
        if total_size.saturating_add(size) > MAX_TOTAL_ALLOCATED_SIZE {
            return ptr::null_mut();
        }
        // The span is raw storage for an object whose bytes the caller writes,
        // so it is asked for zeroed rather than built out of a vector.
        let ptr = if size == 0 {
            // A zero-sized object is still recorded, and still answers with a
            // non-null address, so it gets the dangling pointer of the span
            // alignment: allocating a zero-sized layout is undefined behaviour.
            NonNull::<u128>::dangling().as_ptr() as *mut ()
        } else {
            let Ok(layout) = Layout::from_size_align(size, ALLOCATION_ALIGNMENT) else {
                return ptr::null_mut();
            };
            // SAFETY: the layout is non-zero sized and carries the alignment
            // `free` hands the span back with.
            let ptr = unsafe { std::alloc::alloc_zeroed(layout) };
            if ptr.is_null() {
                return ptr::null_mut();
            }
            ptr as *mut ()
        };
        destinations.insert(
            ptr as usize,
            AllocatedObject {
                span: Span { ptr, size },
                kind: AllocationKind::Heap,
                marked: false,
            },
        );
        *total_size += size;
        ptr
    }

    fn allocate_pages(&mut self, size: usize) -> *mut () {
        let num_pages = size.div_ceil(PAGE_SIZE);
        let ptr = self.pager.allocate(num_pages);
        self.allocated_objects.insert(
            ptr as usize,
            AllocatedObject {
                span: Span {
                    ptr,
                    size: PAGE_SIZE * num_pages,
                },
                kind: AllocationKind::Pages,
                marked: false,
            },
        );
        ptr
    }

    fn free(&mut self, object: &AllocatedObject) {
        match object.kind {
            AllocationKind::Heap => {
                self.total_size = self
                    .total_size
                    .checked_sub(object.span.size)
                    .expect("total allocated size underflow while freeing an object");
                // A zero-sized span has no storage to hand back.
                if object.span.size > 0 {
                    unsafe {
                        std::alloc::dealloc(
                            object.span.ptr as *mut u8,
                            Layout::from_size_align_unchecked(
                                object.span.size,
                                ALLOCATION_ALIGNMENT,
                            ),
                        );
                    }
                }
            }
            AllocationKind::Pages => {
                self.pager
                    .deallocate(object.span.ptr, object.span.size / PAGE_SIZE);
            }
        }
    }

    fn sweep(&mut self) {
        let objects = mem::take(&mut self.allocated_objects);
        let kept: BTreeMap<usize, AllocatedObject> = objects
            .into_iter()
            .filter_map(|(address, object)| {
                if object.marked {
                    Some((address, object))
                } else {
                    self.free(&object);
                    None
                }
            })
            .collect();
        self.allocated_objects = kept;
        let closures = mem::take(&mut self.allocated_closures);
        let kept_closures: BTreeMap<usize, AllocatedObject> = closures
            .into_iter()
            .filter_map(|(address, object)| {
                if object.marked {
                    Some((address, object))
                } else {
                    self.free(&object);
                    None
                }
            })
            .collect();
        self.allocated_closures = kept_closures;

        // The self-describing registries only make sense while the object that
        // owns the descriptor is still alive.
        self.allocated_channels
            .retain(|address, _| self.allocated_objects.contains_key(address));
        self.allocated_maps
            .retain(|address| self.allocated_objects.contains_key(address));
    }

    fn register_global_object(&mut self, address: *mut (), size: usize, type_id: TypeId) {
        self.global_spans.push(GlobalSpan {
            ptr: address,
            size,
            type_id,
        });
    }

    fn free_all_allocated_objects(&mut self) {
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        for object in self.allocated_closures.values_mut() {
            object.marked = false;
        }
        self.sweep();
        assert!(
            self.total_size == 0,
            "total allocated size {} did not reach zero after freeing all objects",
            self.total_size
        );
    }

    pub(crate) fn run_gc(&mut self, contexts: &[&LightWeightThreadContext]) {
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        for object in self.allocated_closures.values_mut() {
            object.marked = false;
        }
        let global_spans = mem::take(&mut self.global_spans);
        for span in &global_spans {
            if span.type_id != TypeId::new_invalid() {
                self.mark_value(span.type_id, span.ptr as usize);
            } else {
                self.mark_range_raw(span.ptr as usize, span.ptr as usize + span.size);
            }
        }
        self.global_spans = global_spans;
        for context in contexts {
            self.mark_stack(context);
        }
        self.sweep();
    }

    /// Marks everything reachable from one goroutine's stack. The live frames
    /// are walked from the innermost one down to the bottom frame and each one
    /// is scanned through its own stack map, so only the slots that can hold a
    /// heap pointer are looked at.
    fn mark_stack(&mut self, context: &LightWeightThreadContext) {
        // The stack region is not an allocation of the heap: its pages come
        // from the pager and the goroutine that ran on it gives them back when
        // it dies, so there is nothing to keep alive here.
        for frame in context.stack_frames() {
            if let Some(get_member_offset_runs) = frame.frame_type.get_member_offset_runs() {
                let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
                get_member_offset_runs(mark_typed_visitor, frame.address, inner);
                continue;
            }
            // No stack map: fall back to a conservative scan of the frame.
            let end = if frame.frame_size > 0 {
                frame.address + frame.frame_size
            } else {
                // Unknown extent: scan the remainder of the stack region so
                // that nothing reachable is missed.
                context.stack_end()
            };
            self.mark_range_raw(frame.address, end);
        }
    }

    fn object_end(&self, object_address: usize) -> Option<usize> {
        self.allocated_objects
            .get(&object_address)
            .map(|object| object.span.ptr as usize + object.span.size)
    }

    /// Marks every object directly reachable through the raw words of
    /// [start, end) and traces them conservatively: a plain heap object is
    /// rescanned word by word, while closures/channels/maps/defer entries use
    /// their own descriptor. This is the keep-alive path used for frame
    /// fallbacks (runtime frames) and untyped contexts.
    fn mark_range_raw(&mut self, start: usize, end: usize) {
        let word_size = mem::size_of::<usize>();
        let pair_size = 2 * word_size;
        let mut address = start;
        // The heap hands out every span 16-byte aligned, so the bulk of a range
        // is walked as pairs of aligned words: an unaligned word is read one
        // byte at a time, which is most of what a collection of a large span
        // costs, and a pair of null words keeps nothing alive, so a span that
        // is mostly zeros is stepped over instead of asked about word by word.
        while address.is_multiple_of(word_size) && address + pair_size <= end {
            let pair = unsafe { ptr::read(address as *const [usize; 2]) };
            if pair != [0; 2] {
                self.mark_word(pair[0]);
                self.mark_word(pair[1]);
            }
            address += pair_size;
        }
        while address + word_size <= end {
            let word = if address.is_multiple_of(word_size) {
                unsafe { ptr::read(address as *const usize) }
            } else {
                unsafe { ptr::read_unaligned(address as *const usize) }
            };
            self.mark_word(word);
            address += word_size;
        }
    }

    /// Marks a single raw word and everything it resolves to.
    fn mark_raw_word(&mut self, address: usize) {
        let word = unsafe { ptr::read_unaligned(address as *const usize) };
        self.mark_word(word);
    }

    /// Marks the object a raw word points at. A null word is nothing to mark:
    /// it is not a tagged closure and lies in no allocation, so a span of
    /// zeros is answered without asking the maps about every one of its words.
    fn mark_word(&mut self, word: usize) {
        if word == 0 {
            return;
        }
        if (word & FUNCTION_OBJECT_CLOSURE_FLAG) != 0 {
            if let Some(closure_address) =
                self.containing_closure(word & !FUNCTION_OBJECT_CLOSURE_FLAG)
            {
                self.mark_object(closure_address);
            }
        } else if let Some(object_address) = self.resolve_any(word) {
            self.mark_object(object_address);
        }
    }

    /// Marks the object a heap word points at, if any. `containing_object`
    /// covers ordinary allocations and `containing_closure` covers interiors
    /// of closure objects (a frame's `free_vars` and interface receivers).
    fn resolve_any(&self, address: usize) -> Option<usize> {
        self.containing_object(address)
            .or_else(|| self.containing_closure(address))
    }

    /// Traces a value of Go type `typ` stored at `addr`. This is the typed
    /// path of the root-driven scan: the type is handed down from stack maps,
    /// globals, slice headers, map entries and interfaces.
    fn mark_value(&mut self, typ: TypeId, addr: usize) {
        match typ.kind() {
            TypeKind::None | TypeKind::NoPointer => {}
            TypeKind::String => self.mark_raw_word(addr),
            TypeKind::Pointer => {
                let pointed_to = typ.pointed_to();
                self.mark_pointer_field(addr, pointed_to);
            }
            TypeKind::Slice => self.mark_slice_header(addr, typ.pointed_to()),
            TypeKind::Interface => self.mark_interface_value(addr),
            TypeKind::Function => self.mark_function_field(addr),
            TypeKind::Map => self.mark_map_slot(addr),
            TypeKind::StructArray => self.mark_struct_array(addr, typ),
        }
    }

    fn mark_pointer_field(&mut self, addr: usize, pointed_to: TypeId) {
        let word = unsafe { ptr::read_unaligned(addr as *const usize) };
        if word == 0 {
            return;
        }
        if (word & FUNCTION_OBJECT_CLOSURE_FLAG) != 0 {
            if let Some(closure_address) =
                self.containing_closure(word & !FUNCTION_OBJECT_CLOSURE_FLAG)
            {
                self.mark_object(closure_address);
            }
            return;
        }
        if let Some(object_address) = self.resolve_any(word) {
            self.mark_object(object_address);
            if pointed_to != TypeId::new_invalid() {
                self.mark_value_at(word, pointed_to);
            }
        }
    }

    /// Marks the value of type `typ` whose storage starts at `address`.
    ///
    /// `address` is the target of a pointer, so it is usually but not always
    /// the base of the allocation holding it: it can also be an interior
    /// address (`&s.field`, `&a[i]`) or a goroutine stack address. The value
    /// is therefore only read when it fits inside the allocation it lives in,
    /// and the rest of that allocation is scanned conservatively otherwise.
    fn mark_value_at(&mut self, address: usize, typ: TypeId) {
        let size = value_size(typ);
        if size == 0 {
            return;
        }
        match self.allocation_end(address) {
            Some(end) if address + size <= end => self.mark_value(typ, address),
            Some(end) => self.mark_range_raw(address, end),
            None => {}
        }
    }

    /// The end of the allocation (heap object, closure, or goroutine stack)
    /// that contains `address`, if any.
    fn allocation_end(&self, address: usize) -> Option<usize> {
        if let Some(base) = self.containing_object(address) {
            return self.object_end(base);
        }
        if let Some(base) = self.containing_closure(address) {
            return self
                .allocated_closures
                .get(&base)
                .map(|object| object.span.ptr as usize + object.span.size);
        }
        None
    }

    /// Marks the backing buffer of a slice whose header is at `addr` and
    /// scans the accessible prefix [data, data + len * size(elem)) element by
    /// element with the element type. Elements beyond `len` are never looked
    /// at, which is what bounds the scan by the length known at trace time.
    fn mark_slice_header(&mut self, addr: usize, elem_type: TypeId) {
        let word_size = mem::size_of::<usize>();
        let data = unsafe { ptr::read_unaligned(addr as *const usize) };
        let len = unsafe { ptr::read_unaligned((addr + word_size) as *const usize) };
        if data == 0 || len == 0 {
            return;
        }
        // Only a buffer that is an allocation of its own is traced from here.
        // A buffer inside the goroutine stack is already reached through the
        // frame slot of the `*[N]T` temporary that made it, a buffer in static
        // data holds no pointers at all, and a buffer that is no allocation is
        // a frame slot that was never written (frames are reused), which must
        // not be interpreted as a slice.
        let Some(object_address) = self.containing_object(data) else {
            return;
        };
        // Keep the buffer itself alive; its interior is scanned precisely
        // below, so it does not get a full-span rescan as well.
        self.mark_object_without_interior(object_address);
        let elem_size = if elem_type == TypeId::new_invalid() {
            word_size
        } else {
            elem_type.size()
        };
        if elem_size == 0 {
            return;
        }
        let span = len.saturating_mul(elem_size);
        let mut end = data.saturating_add(span);
        // The buffer never holds more than its own allocation, so the
        // accessible prefix is clamped to it.
        if let Some(allocation_end) = self.object_end(object_address) {
            end = end.min(allocation_end);
        }
        if end <= data {
            return;
        }
        self.scan_value_range(elem_type, data, end);
    }

    /// Scans [start, end) as a run of values of type `elem`.
    fn scan_value_range(&mut self, elem: TypeId, start: usize, end: usize) {
        match elem.kind() {
            TypeKind::None | TypeKind::NoPointer => {}
            TypeKind::String => {
                let mut addr = start;
                while addr < end {
                    self.mark_raw_word(addr);
                    addr += STRING_SIZE;
                }
            }
            TypeKind::Pointer => {
                let pointed_to = elem.pointed_to();
                let mut addr = start;
                while addr < end {
                    self.mark_pointer_field(addr, pointed_to);
                    addr += mem::size_of::<usize>();
                }
            }
            TypeKind::Slice => {
                let elem_of_elem = elem.pointed_to();
                let mut addr = start;
                while addr < end {
                    self.mark_slice_header(addr, elem_of_elem);
                    addr += SLICE_HEADER_SIZE;
                }
            }
            TypeKind::Interface => {
                let mut addr = start;
                while addr < end {
                    self.mark_interface_value(addr);
                    addr += mem::size_of::<Interface>();
                }
            }
            TypeKind::Function => {
                let mut addr = start;
                while addr < end {
                    self.mark_function_field(addr);
                    addr += mem::size_of::<usize>();
                }
            }
            TypeKind::Map => {
                let mut addr = start;
                while addr < end {
                    self.mark_map_slot(addr);
                    addr += mem::size_of::<usize>();
                }
            }
            TypeKind::StructArray => {
                if let Some(get_member_offset_runs) = elem.get_member_offset_runs() {
                    let step = elem.size();
                    let mut addr = start;
                    while addr < end {
                        let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
                        get_member_offset_runs(mark_typed_visitor, addr, inner);
                        addr = addr.saturating_add(step);
                    }
                } else {
                    self.mark_range_raw(start, end);
                }
            }
        }
    }

    fn mark_interface_value(&mut self, addr: usize) {
        let word_size = mem::size_of::<usize>();
        let receiver = unsafe { ptr::read_unaligned(addr as *const usize) };
        if receiver == 0 {
            return;
        }
        if (receiver & FUNCTION_OBJECT_CLOSURE_FLAG) != 0 {
            if let Some(closure_address) =
                self.containing_closure(receiver & !FUNCTION_OBJECT_CLOSURE_FLAG)
            {
                self.mark_object(closure_address);
            }
            return;
        }
        let Some(object_address) = self.resolve_any(receiver) else {
            return;
        };
        self.mark_object(object_address);
        let type_word = unsafe { ptr::read_unaligned((addr + word_size) as *const usize) };
        // The type word is only a type word if the value really is an
        // interface; a slot the collector was told holds one can still be
        // read before it was written, so it is validated before use.
        let concrete = TypeId::from_raw(type_word);
        if concrete.is_valid() {
            self.mark_value_at(receiver, concrete);
        }
    }

    fn mark_function_field(&mut self, addr: usize) {
        let word = unsafe { ptr::read_unaligned(addr as *const usize) };
        if (word & FUNCTION_OBJECT_CLOSURE_FLAG) != 0
            && let Some(closure_address) =
                self.containing_closure(word & !FUNCTION_OBJECT_CLOSURE_FLAG)
        {
            self.mark_object(closure_address);
        }
    }

    /// Marks the MapObject referenced by a `map[..]` value at `addr`.
    fn mark_map_slot(&mut self, addr: usize) {
        let word = unsafe { ptr::read_unaligned(addr as *const usize) };
        if word == 0 {
            return;
        }
        if let Some(object_address) = self.containing_object(word) {
            self.mark_object(object_address);
        }
    }

    fn mark_struct_array(&mut self, addr: usize, typ: TypeId) {
        if let Some(get_member_offset_runs) = typ.get_member_offset_runs() {
            let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
            get_member_offset_runs(mark_typed_visitor, addr, inner);
        } else {
            let end = addr.saturating_add(typ.size());
            self.mark_range_raw(addr, end);
        }
    }

    fn mark_object(&mut self, object_address: usize) {
        let record = match self.lookup_record(object_address) {
            Some(record) => record,
            None => return,
        };
        if record.marked {
            return;
        }
        self.mark_object_as_marked(object_address);
        match record.special {
            SpecialObject::None => {
                self.mark_range_raw(
                    record.span.ptr as usize,
                    record.span.ptr as usize + record.span.size,
                );
            }
            SpecialObject::Closure => self.mark_closure_scan(record.span.ptr as usize),
            SpecialObject::Channel => self.mark_channel_scan(record.span.ptr as usize),
            SpecialObject::Map => self.mark_map_scan(record.span.ptr as usize),
        }
    }

    /// Marks an object without scanning its interior, used when the referrer
    /// (a slice header) has already traced the exact accessible region.
    fn mark_object_without_interior(&mut self, object_address: usize) {
        let record = match self.lookup_record(object_address) {
            Some(record) => record,
            None => return,
        };
        if record.marked {
            return;
        }
        self.mark_object_as_marked(object_address);
        // The interior is deliberately not scanned: the caller (a slice
        // header) has already traced the exact accessible prefix.
        let _ = record;
    }

    fn mark_closure_scan(&mut self, base: usize) {
        // ClosureLayout { capture_type: TypeId, func: UserFunction, object_ptrs: WordChunk }
        // The captured values are copied right after the layout, starting at
        // base + sizeof(ClosureLayout).
        let word_size = mem::size_of::<usize>();
        let capture_type_word = unsafe {
            ptr::read_unaligned(
                (base + mem::offset_of!(ClosureLayout, capture_type)) as *const usize,
            )
        };
        let capture_start = base + mem::size_of::<ClosureLayout>();
        let capture_type = TypeId::from_raw(capture_type_word);
        if capture_type != TypeId::new_invalid()
            && let Some(get_member_offset_runs) = capture_type.get_member_offset_runs()
        {
            let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
            get_member_offset_runs(mark_typed_visitor, capture_start, inner);
            return;
        }
        // Untyped captures: the extent is the WordChunk count at object_ptrs.
        let count = unsafe {
            ptr::read_unaligned(
                (base + mem::offset_of!(ClosureLayout, object_ptrs)) as *const usize,
            )
        };
        let end = capture_start.saturating_add(count.saturating_mul(word_size));
        self.mark_range_raw(capture_start, end);
    }

    fn mark_channel_scan(&mut self, base: usize) {
        let elem_type = self
            .allocated_channels
            .get(&base)
            .copied()
            .unwrap_or(TypeId::new_invalid());
        let channel = unsafe { &*(base as *const ChannelObject) };
        // A channel object holds exactly one pointer of its own, the ring array
        // of a buffered channel; scanning the object's words instead would read
        // the padding behind `is_closed`.
        if let Some(buffer) = channel.buffer()
            && let Some(object_address) = self.containing_object(buffer)
        {
            self.mark_object(object_address);
        }
        channel.gc_keep(|object_ptr| {
            let address = object_ptr.0 as usize;
            if let Some(object_address) = self.containing_object(address) {
                self.mark_object(object_address);
                if elem_type != TypeId::new_invalid() {
                    self.mark_value_at(address, elem_type);
                }
            }
        });
    }

    fn mark_map_scan(&mut self, base: usize) {
        // Keep the table the map owns: it is allocated through this allocator
        // and no other word leads to it. The map records an address inside it,
        // because the hash table header has bytes that are never written and
        // scanning the struct itself would read them.
        let map = unsafe { &*(base as *const MapObject) };
        if let Some(address) = map.table_address()
            && let Some(object_address) = self.containing_object(address)
        {
            self.mark_object(object_address);
        }
        // The boxed keys and values live in their own allocations, so they are
        // reached through the entries rather than through the table itself.
        map.gc_keep(|key_type, key_address, value_type, value_address| {
            self.mark_box_with_type(key_address, key_type);
            self.mark_box_with_type(value_address, value_type);
        });
    }

    /// Marks a boxed value of type `typ` (resolving `address` to the box).
    fn mark_box_with_type(&mut self, address: usize, typ: TypeId) {
        if let Some(object_address) = self.containing_object(address) {
            self.mark_object(object_address);
            if typ != TypeId::new_invalid() {
                self.mark_value_at(address, typ);
            }
        }
    }

    fn containing_object(&self, address: usize) -> Option<usize> {
        let (object_address, object) = self.allocated_objects.range(..=address).next_back()?;
        let object_address = *object_address;
        if address < object_address + object.span.size {
            Some(object_address)
        } else {
            None
        }
    }

    fn containing_closure(&self, address: usize) -> Option<usize> {
        let (closure_address, closure) = self.allocated_closures.range(..=address).next_back()?;
        let closure_address = *closure_address;
        if address < closure_address + closure.span.size {
            Some(closure_address)
        } else {
            None
        }
    }

    fn lookup_record(&self, object_address: usize) -> Option<MarkRecord> {
        if let Some(object) = self.allocated_objects.get(&object_address) {
            let special = if self.allocated_channels.contains_key(&object_address) {
                SpecialObject::Channel
            } else if self.allocated_maps.contains(&object_address) {
                SpecialObject::Map
            } else {
                SpecialObject::None
            };
            return Some(MarkRecord {
                span: object.span,
                marked: object.marked,
                special,
            });
        }
        if let Some(object) = self.allocated_closures.get(&object_address) {
            return Some(MarkRecord {
                span: object.span,
                marked: object.marked,
                special: SpecialObject::Closure,
            });
        }
        None
    }

    fn mark_object_as_marked(&mut self, object_address: usize) {
        if let Some(object) = self.allocated_objects.get_mut(&object_address) {
            object.marked = true;
        } else if let Some(object) = self.allocated_closures.get_mut(&object_address) {
            object.marked = true;
        }
    }
}

/// Placeholder so the compiler and readers keep the strided sizes consistent
/// with the C lay-out: a slice header is 3 words, a string is 2 words.
/// A snapshot of an allocation taken before it is marked, so that the mark
/// can continue without holding a borrow of the allocation map.
#[derive(Clone, Copy)]
struct MarkRecord {
    span: Span,
    marked: bool,
    special: SpecialObject,
}

impl ObjectAllocatorInner {
    fn mark_typed_offset(&mut self, offset: usize, size: usize, typ: TypeId, kind: i32) {
        let word_size = mem::size_of::<usize>();
        match kind {
            GC_SLOT_POINTER => {
                let pointed_to = typ;
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + word_size <= end {
                    self.mark_pointer_field(address, pointed_to);
                    address += word_size;
                }
            }
            GC_SLOT_SLICE => {
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + SLICE_HEADER_SIZE <= end {
                    self.mark_slice_header(address, typ);
                    address += SLICE_HEADER_SIZE;
                }
            }
            GC_SLOT_INTERFACE => {
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + mem::size_of::<Interface>() <= end {
                    self.mark_interface_value(address);
                    address += mem::size_of::<Interface>();
                }
            }
            GC_SLOT_STRING => {
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + STRING_SIZE <= end {
                    self.mark_raw_word(address);
                    address += STRING_SIZE;
                }
            }
            GC_SLOT_FUNCTION => {
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + word_size <= end {
                    self.mark_function_field(address);
                    address += word_size;
                }
            }
            GC_SLOT_DEFER_STACK => {
                let word = unsafe { ptr::read_unaligned(offset as *const usize) };
                if let Some(object_address) = self.resolve_any(word) {
                    self.mark_object(object_address);
                }
            }
            GC_SLOT_FREE_VARS => {
                let word = unsafe { ptr::read_unaligned(offset as *const usize) };
                if word != 0
                    && let Some(closure_address) = self.containing_closure(word)
                {
                    self.mark_object(closure_address);
                }
            }
            GC_SLOT_RAW => self.mark_range_raw(offset, offset + size),
            // An unknown slot kind is scanned conservatively rather than
            // skipped: the whole point of a raw run is that nothing is known.
            _ => self.mark_range_raw(offset, offset + size),
        }
    }
}

/// Visitor passed to a generated get_member_offset_runs function: it marks the
/// reported typed slot. `kind` is a GCSlotKind and `typ` its accompanying
/// TypeId (the pointee for pointer runs, the element type for slice runs).
extern "C" fn mark_typed_visitor(
    offset: usize,
    size: usize,
    typ: TypeId,
    kind: i32,
    arg: *mut ffi::c_void,
) {
    let inner = unsafe { &mut *(arg as *mut ObjectAllocatorInner) };
    inner.mark_typed_offset(offset, size, typ, kind);
}

// Keep the compiler honest about the visitor signature (a null fn item). The
// real signature is declared in type_id.rs and mirrored in predefined.h.
const _: TypeOffsetVisitor = mark_typed_visitor;

#[derive(Clone)]
pub(crate) struct ObjectAllocatorPtr(*mut ObjectAllocatorInner);

impl ObjectAllocatorPtr {
    pub(crate) fn allocate(&self, size: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate(size)
    }

    pub(crate) fn allocate_channel(&self, size: usize, elem_type: TypeId) -> *mut () {
        unsafe { &mut *self.0 }.allocate_channel(size, elem_type)
    }

    pub(crate) fn allocate_closure(&self, size: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate_closure(size)
    }

    pub(crate) fn allocate_map(&self, size: usize) -> *mut () {
        unsafe { &mut *self.0 }.allocate_map(size)
    }

    pub(crate) fn register_global_object(&self, address: *mut (), size: usize, type_id: TypeId) {
        unsafe { &mut *self.0 }.register_global_object(address, size, type_id);
    }

    pub(crate) fn run_gc(&self, contexts: &[&LightWeightThreadContext]) {
        unsafe { &mut *self.0 }.run_gc(contexts)
    }

    #[cfg(test)]
    pub(crate) fn global_spans_len(&self) -> usize {
        unsafe { &*self.0 }.global_spans.len()
    }

    #[cfg(test)]
    pub(crate) fn total_size(&self) -> usize {
        unsafe { &*self.0 }.total_size
    }

    #[cfg(test)]
    pub(crate) fn contains(&self, ptr: *mut ()) -> bool {
        unsafe { &*self.0 }
            .allocated_objects
            .contains_key(&(ptr as usize))
    }

    #[cfg(test)]
    pub(crate) fn contains_closure(&self, ptr: *mut ()) -> bool {
        unsafe { &*self.0 }
            .allocated_closures
            .contains_key(&(ptr as usize))
    }

    #[cfg(test)]
    pub(crate) fn registered_channels_len(&self) -> usize {
        unsafe { &*self.0 }.allocated_channels.len()
    }

    #[cfg(test)]
    pub(crate) fn registered_maps_len(&self) -> usize {
        unsafe { &*self.0 }.allocated_maps.len()
    }
}

unsafe impl Allocator for ObjectAllocatorPtr {
    fn allocate(&self, layout: Layout) -> Result<NonNull<[u8]>, AllocError> {
        let ptr = if layout.align() <= ALLOCATION_ALIGNMENT {
            let ptr = self.allocate(layout.size());
            if ptr.is_null() {
                return Err(AllocError);
            }
            ptr
        } else {
            let total = layout.size() + layout.align();
            let base = self.allocate(total);
            if base.is_null() {
                return Err(AllocError);
            }
            let base = base as usize;
            let offset = (base as *const u8).align_offset(layout.align());
            assert!(offset < layout.align());
            (base + offset) as *mut ()
        };
        let slice = ptr::slice_from_raw_parts_mut(ptr as *mut u8, layout.size());
        unsafe { Ok(NonNull::new_unchecked(slice)) }
    }

    unsafe fn deallocate(&self, _ptr: NonNull<u8>, _layout: Layout) {}
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::FunctionObject;
    use crate::ObjectPtr;
    use crate::StackFrameCommon;
    use crate::allocator::ObjectAllocator;

    /// Where a generated frame keeps its first local: just past the header
    /// every frame starts with. The stack map generators below list their
    /// member there, so a test that fills a listed member must not scribble on
    /// the header, whose words the frame walk itself reads.
    const FRAME_LOCAL_OFFSET: usize = mem::size_of::<StackFrameCommon>();
    const FRAME_LOCAL_WORD: usize = FRAME_LOCAL_OFFSET / mem::size_of::<usize>();
    use crate::UserFunction;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use crate::object::channel::ChannelObject;
    use crate::object::map::MapObject;
    use crate::object::string::StringObject;
    use crate::type_id::FakeTypeInfo;
    use crate::type_id::TYPE_INFO_MAGIC;
    use crate::type_id::TypeInfo;
    use crate::type_id::TypeKind;
    use std::sync::OnceLock;

    /// The pointee type of a `*Node` slot, needed by the generators below
    /// (which cannot capture anything).
    static NODE_TID: OnceLock<TypeId> = OnceLock::new();

    extern "C" fn node_generator(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void) {
        let node = *NODE_TID.get().unwrap();
        visit(base, mem::size_of::<usize>(), node, GC_SLOT_POINTER, arg);
    }

    /// A struct whose only pointer-bearing member is the frame's first local.
    extern "C" fn offset8_generator(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void) {
        let node = *NODE_TID.get().unwrap();
        visit(
            base + FRAME_LOCAL_OFFSET,
            mem::size_of::<usize>(),
            node,
            GC_SLOT_POINTER,
            arg,
        );
    }

    /// A frame-shaped type whose only member is an interface value.
    extern "C" fn interface_frame_generator(
        visit: TypeOffsetVisitor,
        base: usize,
        arg: *mut ffi::c_void,
    ) {
        visit(
            base + FRAME_LOCAL_OFFSET,
            2 * mem::size_of::<usize>(),
            TypeId::new_invalid(),
            GC_SLOT_INTERFACE,
            arg,
        );
    }

    /// A frame-shaped type whose only member is a slice header.
    extern "C" fn slice_frame_generator(
        visit: TypeOffsetVisitor,
        base: usize,
        arg: *mut ffi::c_void,
    ) {
        visit(
            base + FRAME_LOCAL_OFFSET,
            SLICE_HEADER_SIZE,
            *NODE_TID.get().unwrap(),
            GC_SLOT_SLICE,
            arg,
        );
    }

    /// A frame-shaped type whose pointer-bearing member is the defer stack.
    extern "C" fn defer_stack_generator(
        visit: TypeOffsetVisitor,
        base: usize,
        arg: *mut ffi::c_void,
    ) {
        // The defer stack is a member of the frame, wherever it sits in it.
        visit(
            base + mem::offset_of!(crate::StackFrameCommon, defer_stack),
            mem::size_of::<crate::DeferStack>(),
            TypeId::new_invalid(),
            GC_SLOT_DEFER_STACK,
            arg,
        );
    }

    struct TestTypes {
        /// struct { p *Node }, one word.
        node: FakeTypeInfo,
        /// *struct { p *Node }
        node_ptr: FakeTypeInfo,
        /// An opaque pointer (a channel value, or a pointer to something whose
        /// interior must not be traced).
        opaque_ptr: FakeTypeInfo,
        string: FakeTypeInfo,
        interface: FakeTypeInfo,
        function: FakeTypeInfo,
        slice_of_node: FakeTypeInfo,
        /// []*struct { p *Node }
        slice_of_node_ptr: FakeTypeInfo,
        /// struct { opaque, p *Node }: the pointer sits at offset 8.
        offset8: FakeTypeInfo,
        /// A defer-stack frame shape.
        defer_frame: FakeTypeInfo,
        /// A frame shape whose only member is a slice header.
        slice_frame: FakeTypeInfo,
        /// A frame shape whose only member is an interface value.
        interface_frame: FakeTypeInfo,
        /// 16 bytes without any pointer.
        opaque_16: FakeTypeInfo,
        /// *[]*Node, the type of a pointer to a slice header.
        slice_of_node_ptr_ptr: FakeTypeInfo,
    }

    fn types() -> &'static TestTypes {
        static TYPES: OnceLock<TestTypes> = OnceLock::new();
        TYPES.get_or_init(|| {
            let node = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(node_generator),
                16,
            );
            let node_tid = node.tid();
            let _ = NODE_TID.set(node_tid);
            let node_ptr = FakeTypeInfo::pointer(node_tid);
            let opaque_ptr = FakeTypeInfo::pointer(TypeId::new_invalid());
            let string = FakeTypeInfo::string();
            let interface = FakeTypeInfo::interface_();
            let function = FakeTypeInfo::function();
            let slice_of_node = FakeTypeInfo::slice_of(node_tid);
            let slice_of_node_ptr = FakeTypeInfo::slice_of(node_ptr.tid());
            let offset8 = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(offset8_generator),
                16,
            );
            let defer_frame = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(defer_stack_generator),
                16,
            );
            let opaque_16 =
                FakeTypeInfo::new_with(TypeKind::NoPointer, TypeId::new_invalid(), None, 16);
            let slice_of_node_ptr_ptr = FakeTypeInfo::pointer(slice_of_node_ptr.tid());
            let interface_frame = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(interface_frame_generator),
                2 * mem::size_of::<usize>(),
            );
            let slice_frame = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(slice_frame_generator),
                SLICE_HEADER_SIZE,
            );
            TestTypes {
                node,
                node_ptr,
                opaque_ptr,
                string,
                interface,
                function,
                slice_of_node,
                slice_of_node_ptr,
                offset8,
                defer_frame,
                opaque_16,
                slice_of_node_ptr_ptr,
                slice_frame,
                interface_frame,
            }
        })
    }

    unsafe extern "C" fn dummy_func(_ctx: &mut LightWeightThreadContext) -> FunctionObject {
        FunctionObject::new_null()
    }

    fn create_ctx() -> (
        LightWeightThreadContext,
        crate::global_context::GlobalContextPtr,
    ) {
        let gc = global_context::create_global_context();
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    static TEST_KEY_NAME: [u8; 4] = *b"key\0";

    extern "C" fn test_is_equal(a: ObjectPtr, b: ObjectPtr) -> bool {
        unsafe { *(a.0 as *const u64) == *(b.0 as *const u64) }
    }

    extern "C" fn test_hash(a: ObjectPtr) -> usize {
        unsafe { *(a.0 as *const u64) as usize }
    }

    /// A pointer-free 16-byte map key type with working hash/equal functions,
    /// which a map needs in order to look an entry up.
    fn map_key_type() -> TypeId {
        static INSTANCE: OnceLock<TypeInfo> = OnceLock::new();
        let info = INSTANCE.get_or_init(|| TypeInfo {
            gc_magic: TYPE_INFO_MAGIC,
            name: StringObject::new(TEST_KEY_NAME.as_ptr(), 3),
            num_methods: 0,
            interface_table: ptr::null(),
            is_equal: test_is_equal,
            hash: test_hash,
            size: 16,
            kind: TypeKind::NoPointer as i32,
            pointed_to: TypeId::new_invalid(),
            get_member_offset_runs: None,
        });
        TypeId::from_raw(info as *const TypeInfo as usize)
    }

    /// The little-endian bytes of a pointer, as a root slot.
    fn word<T>(value: *const T) -> [u8; 8] {
        (value as usize).to_le_bytes()
    }

    /// The little-endian bytes of an already raw word.
    fn raw_word(value: usize) -> [u8; 8] {
        value.to_le_bytes()
    }

    /// A `struct { p *Node }` object whose first word is `child`.
    fn alloc_node(allocator: &ObjectAllocatorPtr, child: *mut ()) -> *mut () {
        let node = allocator.allocate(16);
        unsafe { (node as *mut usize).write(child as usize) };
        node
    }

    /// The size of every global root slot used by these tests.
    const ROOT_SLOT_SIZE: usize = 32;

    /// Gives the storage of a global root back when it goes out of scope.
    struct RootGuard(*mut [u8; ROOT_SLOT_SIZE]);

    impl Drop for RootGuard {
        fn drop(&mut self) {
            unsafe {
                drop(Box::from_raw(self.0));
            }
        }
    }

    /// Registers a value of type `typ` as a global root. The first word of
    /// `bytes` is the root word (a pointer, a tagged function, ...).
    ///
    /// The storage has to outlive the collection, which the box does; keep the
    /// returned guard alive until the test is done with the root.
    fn register_root(allocator: &ObjectAllocatorPtr, typ: TypeId, bytes: &[u8]) -> RootGuard {
        assert!(bytes.len() <= ROOT_SLOT_SIZE);
        let mut storage = [0u8; ROOT_SLOT_SIZE];
        storage[..bytes.len()].copy_from_slice(bytes);
        let address = Box::leak(Box::new(storage));
        allocator.register_global_object(address.as_mut_ptr() as *mut (), ROOT_SLOT_SIZE, typ);
        RootGuard(address)
    }

    #[test]
    fn test_allocation_above_the_threshold_is_mapped_from_pages() {
        let pager = Rc::new(Pager::new());
        let allocator = ObjectAllocator::new(pager.clone());
        let size = LARGE_ALLOCATION_THRESHOLD + 1;
        let ptr = allocator.ptr().allocate(size);
        assert!(!ptr.is_null());
        // The pages of the allocation belong to the pager that maps them.
        assert_eq!(
            pager.allocated_pages(),
            size.div_ceil(PAGE_SIZE),
            "the pager did not count the pages of the large allocation"
        );
        assert!(allocator.ptr().contains(ptr));
        // Mapped from whole pages between two guard pages, and zeroed like a
        // heap span.
        assert_eq!(ptr as usize % PAGE_SIZE, 0);
        assert_eq!(unsafe { (ptr as *const u8).add(size - 1).read() }, 0);
        #[cfg(not(miri))]
        {
            // The region is rounded up to whole pages, so the byte past the end
            // of the allocation is mapped (and zeroed) as well.
            assert_eq!(unsafe { (ptr as *const u8).add(size).read() }, 0);
        }
        // Its pages are not heap, so they do not eat into the heap budget that
        // refuses an allocation once MAX_TOTAL_ALLOCATED_SIZE is reached.
        assert_eq!(allocator.ptr().total_size(), 0);
    }

    #[test]
    fn test_sweep_unmaps_a_large_allocation() {
        let pager = Rc::new(Pager::new());
        let allocator = ObjectAllocator::new(pager.clone());
        let size = LARGE_ALLOCATION_THRESHOLD + 1;
        let ptr = allocator.ptr().allocate(size);
        allocator.ptr().run_gc(&[]);
        assert!(!allocator.ptr().contains(ptr));
        assert_eq!(pager.allocated_pages(), 0);
        #[cfg(not(miri))]
        assert!(
            sweep_unmaps_a_large_allocation(),
            "mprotect succeeded on the pages of a large allocation that was swept"
        );
    }

    /// Whether the collector really unmaps the pages of an allocation too large
    /// for the heap when it sweeps it. Reading the pages would fault, which a
    /// test cannot catch, so a forked child checks with mprotect, which fails on
    /// an address that is not mapped anymore.
    ///
    /// The child allocates and sweeps the region itself: an address that was
    /// unmapped here is free for another thread to map again, and such a mapping
    /// is what makes the check fail for the wrong reason. The child has its own
    /// address space, in which nothing else runs.
    #[cfg(not(miri))]
    fn sweep_unmaps_a_large_allocation() -> bool {
        let size = LARGE_ALLOCATION_THRESHOLD + 1;
        // SAFETY: the child only allocates, sweeps, calls mprotect and exits.
        let pid = unsafe { libc::fork() };
        assert!(pid >= 0, "fork failed");
        if pid == 0 {
            let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
            let ptr = allocator.ptr().allocate(size);
            allocator.ptr().run_gc(&[]);
            let ret = unsafe {
                libc::mprotect(
                    ptr as *mut libc::c_void,
                    size.div_ceil(PAGE_SIZE) * PAGE_SIZE,
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
    fn test_large_allocation_is_traced_like_a_heap_allocation() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let region = allocator.ptr().allocate(LARGE_ALLOCATION_THRESHOLD + 1);
        // The pointer to the leaf sits in the middle of the region, so it is
        // only found if the interior of a page-backed allocation is scanned.
        unsafe { (region as *mut usize).add(64).write(leaf as usize) };
        let _root = register_root(&allocator.ptr(), TypeId::new_invalid(), &word(region));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(region));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_allocate_rounds_size_up_and_registers_object() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let ptr = allocator.ptr().allocate(1);
        assert!(!ptr.is_null());
        assert!(allocator.ptr().contains(ptr));
        assert!(!allocator.ptr().contains_closure(ptr));
    }

    #[test]
    fn test_allocate_returns_null_when_heap_is_exhausted() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        while !allocator.ptr().allocate(65536).is_null() {}
        assert!(allocator.ptr().allocate(65536).is_null());
    }

    #[test]
    fn test_allocate_closure_is_registered_apart_from_ordinary_objects() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let closure = allocator.ptr().allocate_closure(32);
        assert!(!closure.is_null());
        assert!(!allocator.ptr().contains(closure));
        assert!(allocator.ptr().contains_closure(closure));

        let ordinary = allocator.ptr().allocate(32);
        assert!(allocator.ptr().contains(ordinary));
        assert!(!allocator.ptr().contains_closure(ordinary));
    }

    #[test]
    fn test_untyped_frame_is_scanned_conservatively() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let target = alloc_node(&allocator.ptr(), ptr::null_mut());
        let frame_size = FRAME_LOCAL_OFFSET + 2 * mem::size_of::<usize>();
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        unsafe {
            (ctx.stack_pointer() as *mut usize).write(target as usize);
        };

        allocator.ptr().run_gc(&[&ctx]);
        assert!(allocator.ptr().contains(target));
    }

    #[test]
    fn test_typed_global_pointer_traces_transitively() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let child = alloc_node(&allocator.ptr(), leaf);
        let root = alloc_node(&allocator.ptr(), child);
        let _root = register_root(&allocator.ptr(), types().node_ptr.tid(), &word(root));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(root));
        assert!(allocator.ptr().contains(child));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_no_pointer_global_interior_is_not_scanned() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let target = alloc_node(&allocator.ptr(), ptr::null_mut());
        let _root = register_root(&allocator.ptr(), types().opaque_16.tid(), &word(target));

        allocator.ptr().run_gc(&[]);
        assert!(!allocator.ptr().contains(target));
    }

    #[test]
    fn test_string_global_keeps_its_buffer() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let buffer = allocator.ptr().allocate(8);
        unsafe { (buffer as *mut u8).write(0) };
        let mut value = [0u8; 16];
        value[..8].copy_from_slice(&word(buffer));
        value[8..16].copy_from_slice(&4usize.to_le_bytes());
        let _root = register_root(&allocator.ptr(), types().string.tid(), &value);

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(buffer));
    }

    #[test]
    fn test_slice_global_scans_only_the_accessible_prefix() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let len: usize = 2;
        let cap: usize = 4;
        let buffer = allocator.ptr().allocate(cap * mem::size_of::<usize>());
        let alive = alloc_node(&allocator.ptr(), ptr::null_mut());
        let beyond_len = alloc_node(&allocator.ptr(), ptr::null_mut());
        unsafe {
            (buffer as *mut usize).write(alive as usize);
            ((buffer as *mut usize).add(2)).write(beyond_len as usize);
        };

        let mut header = [0u8; 24];
        header[..8].copy_from_slice(&word(buffer));
        header[8..16].copy_from_slice(&len.to_le_bytes());
        header[16..24].copy_from_slice(&cap.to_le_bytes());
        let _root = register_root(&allocator.ptr(), types().slice_of_node_ptr.tid(), &header);

        allocator.ptr().run_gc(&[]);
        // The buffer and the elements below len stay alive ...
        assert!(allocator.ptr().contains(buffer));
        assert!(allocator.ptr().contains(alive));
        // ... while the element above len is not reachable through the slice.
        assert!(!allocator.ptr().contains(beyond_len));
    }

    #[test]
    fn test_slice_of_struct_elements_traces_their_members() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let buffer = allocator.ptr().allocate(16);
        unsafe { (buffer as *mut usize).write(leaf as usize) };

        let mut header = [0u8; 24];
        header[..8].copy_from_slice(&word(buffer));
        header[8..16].copy_from_slice(&1usize.to_le_bytes());
        header[16..24].copy_from_slice(&1usize.to_le_bytes());
        let _root = register_root(&allocator.ptr(), types().slice_of_node.tid(), &header);

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(buffer));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_interface_global_marks_receiver_and_its_members() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let box_ptr = alloc_node(&allocator.ptr(), leaf);
        let mut value = [0u8; 16];
        value[..8].copy_from_slice(&word(box_ptr));
        value[8..16].copy_from_slice(&raw_word(types().node.tid().to_raw()));
        let _root = register_root(&allocator.ptr(), types().interface.tid(), &value);

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(box_ptr));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_interface_global_with_pointer_free_receiver() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let box_ptr = allocator.ptr().allocate(8);
        let mut value = [0u8; 16];
        value[..8].copy_from_slice(&word(box_ptr));
        value[8..16].copy_from_slice(&raw_word(types().opaque_16.tid().to_raw()));
        let _root = register_root(&allocator.ptr(), types().interface.tid(), &value);

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(box_ptr));
    }

    #[test]
    fn test_function_global_marks_closure_with_typed_captures() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let captured = alloc_node(&allocator.ptr(), leaf);
        let layout = allocator
            .ptr()
            .allocate_closure(mem::size_of::<ClosureLayout>() + mem::size_of::<usize>())
            as *mut ClosureLayout;
        unsafe {
            ptr::addr_of_mut!((*layout).capture_type).write(types().node.tid());
            ptr::addr_of_mut!((*layout).func).write(UserFunction::new(dummy_func));
            ptr::addr_of_mut!((*layout).object_ptrs)
                .cast::<usize>()
                .write(1);
            let capture_data = (layout as *mut u8).add(mem::size_of::<ClosureLayout>());
            (capture_data as *mut usize).write(captured as usize);
        };

        let function = FunctionObject::from_closure_layout_ptr(layout as *const ());
        let _root = register_root(
            &allocator.ptr(),
            types().function.tid(),
            &raw_word(function.0 as usize),
        );

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains_closure(layout as *mut ()));
        assert!(allocator.ptr().contains(captured));
        // The captured value is a *Node, so its own member is traced too.
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_channel_scan_keeps_buffered_values() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let ptr = allocator
            .ptr()
            .allocate_channel(mem::size_of::<ChannelObject>(), types().node.tid())
            as *mut ChannelObject;
        unsafe {
            ptr::write(ptr, ChannelObject::new(2, &allocator.ptr()));
        }
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let data = alloc_node(&allocator.ptr(), leaf);
        unsafe {
            let channel = &mut *ptr;
            assert_eq!(channel.send(1, ObjectPtr(data)), Some(()));
        };

        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(ptr));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(data));
        assert!(allocator.ptr().contains(leaf));
        assert_eq!(allocator.ptr().registered_channels_len(), 1);
    }

    #[test]
    fn test_map_scan_keeps_entry_boxes() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let ptr = allocator.ptr().allocate_map(mem::size_of::<MapObject>()) as *mut MapObject;
        MapObject::construct_in(ptr, map_key_type(), types().node.tid(), allocator.ptr());

        let mut key = [0u64; 2];
        key[0] = 7;
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let value = alloc_node(&allocator.ptr(), leaf);
        unsafe {
            let map = &mut *ptr;
            map.set(ObjectPtr(key.as_mut_ptr() as *mut ()), ObjectPtr(value));
        };

        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(ptr));

        allocator.ptr().run_gc(&[]);
        assert_eq!(allocator.ptr().registered_maps_len(), 1);
        assert!(allocator.ptr().contains(ptr as *mut ()));
        // The value box copied by MapObject::set survived the collection, and
        // because the map knows the value type, its member was traced too.
        assert!(allocator.ptr().contains(leaf));

        let mut out = [0u64; 2];
        let map = unsafe { &*ptr };
        assert!(map.get(
            ObjectPtr(key.as_mut_ptr() as *mut ()),
            ObjectPtr(out.as_mut_ptr() as *mut ()),
        ));
        assert_eq!(out[0] as usize, leaf as usize);
    }

    #[test]
    fn test_sweep_prunes_channel_and_map_registrations() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let channel = allocator.ptr().allocate(mem::size_of::<ChannelObject>());
        allocator
            .ptr()
            .allocate_channel(mem::size_of::<ChannelObject>(), types().node.tid());
        let map = allocator.ptr().allocate(mem::size_of::<MapObject>());
        allocator.ptr().allocate_map(mem::size_of::<MapObject>());
        assert_eq!(allocator.ptr().registered_channels_len(), 1);
        assert_eq!(allocator.ptr().registered_maps_len(), 1);

        allocator.ptr().run_gc(&[]);
        assert_eq!(allocator.ptr().registered_channels_len(), 0);
        assert_eq!(allocator.ptr().registered_maps_len(), 0);
        assert!(!allocator.ptr().contains(channel));
        assert!(!allocator.ptr().contains(map));
    }

    /// A pointer does not have to be the base of the allocation it points
    /// into: it can be an interior address, which is how a `*[N]T` frame
    /// temporary of a `make([]T, n)` ends up pointing into the memory that
    /// holds it. Such a region is never scanned conservatively from its base,
    /// so the pointee has to be traced exactly where the pointer points. The
    /// first word of the region is null, which makes a scan from the region's
    /// base find nothing at all.
    fn region_with_value_at(allocator: &ObjectAllocatorPtr, offset: usize) -> *mut usize {
        // The region is a large allocation, so it is mapped from the pager's
        // pages: nothing but a pointer into its interior reaches the value.
        let region = allocator.allocate(LARGE_ALLOCATION_THRESHOLD + 1);
        unsafe { (region as *mut usize).write(0) };
        unsafe { (region as *mut usize).add(offset) }
    }

    #[test]
    fn test_pointer_to_interior_address_traces_the_value_there() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let buffer = allocator.ptr().allocate(mem::size_of::<usize>());
        unsafe { (buffer as *mut usize).write(leaf as usize) };
        // A `[]*Node` header living inside the region instead of in an
        // allocation of its own.
        let header = region_with_value_at(&allocator.ptr(), 256);
        unsafe {
            header.write(buffer as usize);
            header.add(1).write(1);
            header.add(2).write(1);
        };
        let _root = register_root(
            &allocator.ptr(),
            types().slice_of_node_ptr_ptr.tid(),
            &word(header),
        );

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(buffer));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_pointer_to_interior_struct_field_traces_its_members() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        // A `*Node` pointing at a node that is embedded in the region: the
        // node generator then finds the leaf at its own first word.
        let inner = region_with_value_at(&allocator.ptr(), 256);
        unsafe { inner.write(leaf as usize) };
        let _root = register_root(&allocator.ptr(), types().node_ptr.tid(), &word(inner));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(leaf));
    }

    /// Pushes a frame whose only listed member is a slice header, and fills
    /// that header with `data` and `len`.
    fn push_slice_frame(ctx: &mut LightWeightThreadContext, data: usize, len: usize) {
        let frame_size = FRAME_LOCAL_OFFSET + SLICE_HEADER_SIZE;
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        unsafe {
            let frame = (ctx.stack_pointer() as *mut usize).add(FRAME_LOCAL_WORD);
            frame.write(data);
            frame.add(1).write(len);
            frame.add(2).write(len);
        }
        ctx.stack_frame_mut::<StackFrameCommon>().frame_type = types().slice_frame.tid();
    }

    /// Pushes a frame whose only listed member is an interface value, holding
    /// `receiver` and `type_word`.
    fn push_interface_frame(ctx: &mut LightWeightThreadContext, receiver: usize, type_word: usize) {
        ctx.push_frame(
            FRAME_LOCAL_OFFSET + 2 * mem::size_of::<usize>(),
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        unsafe {
            let frame = (ctx.stack_pointer() as *mut usize).add(FRAME_LOCAL_WORD);
            frame.write(receiver);
            frame.add(1).write(type_word);
        }
        ctx.stack_frame_mut::<StackFrameCommon>().frame_type = types().interface_frame.tid();
    }

    #[test]
    fn test_interface_slot_with_a_garbage_type_word_keeps_the_receiver() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let receiver = alloc_node(&allocator.ptr(), ptr::null_mut());
        // Frames are reused, so an interface-typed slot can still hold the
        // words of an older call. The receiver is still a live object and has
        // to be kept; the type word is not a TypeId and must not be used.
        push_interface_frame(&mut ctx, receiver as usize, 0x5d64936eccaa);

        allocator.ptr().run_gc(&[&ctx]);
        assert!(allocator.ptr().contains(receiver));
    }

    #[test]
    fn test_slice_slot_of_unwritten_frame_is_ignored() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        // Frames are reused, so a slot can still hold the words of an older
        // call. A length read from such a slot must not be believed.
        push_slice_frame(&mut ctx, 0x1_0000_0000, 1 << 40);

        allocator.ptr().run_gc(&[&ctx]);
    }

    #[test]
    fn test_slice_scan_is_clamped_to_the_buffer_allocation() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let buffer = alloc_node(&allocator.ptr(), ptr::null_mut());
        let beyond = alloc_node(&allocator.ptr(), ptr::null_mut());
        // The length claims far more elements than the 16-byte allocation
        // holds, so only the allocation itself may be scanned.
        push_slice_frame(&mut ctx, buffer as usize, 512);

        allocator.ptr().run_gc(&[&ctx]);
        assert!(allocator.ptr().contains(buffer));
        assert!(!allocator.ptr().contains(beyond));
    }

    #[test]
    fn test_typed_stack_frame_scans_only_listed_slots() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let listed = alloc_node(&allocator.ptr(), ptr::null_mut());
        let unlisted = alloc_node(&allocator.ptr(), ptr::null_mut());
        let frame_size = 2 * mem::size_of::<usize>();
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        unsafe {
            let frame = (ctx.stack_pointer() as *mut usize).add(FRAME_LOCAL_WORD);
            // The first local is the pointer-bearing member the frame type
            // lists, the one after it is not listed.
            frame.write(listed as usize);
            frame.add(1).write(unlisted as usize);
        }
        ctx.stack_frame_mut::<StackFrameCommon>().frame_type = types().offset8.tid();

        allocator.ptr().run_gc(&[&ctx]);
        assert!(allocator.ptr().contains(listed));
        assert!(!allocator.ptr().contains(unlisted));
    }

    #[test]
    fn test_defer_stack_slot_keeps_the_entry_and_its_arguments() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let argument = alloc_node(&allocator.ptr(), ptr::null_mut());
        // A defer stack entry: next link, function, and the argument words of
        // the deferred call.
        let entry = allocator.ptr().allocate(3 * mem::size_of::<usize>());
        unsafe {
            (entry as *mut usize).write(0);
            ((entry as *mut usize).add(1)).write(0);
            ((entry as *mut usize).add(2)).write(argument as usize);
        };

        // The bottom frame links to itself, so the walk ends on it, and the
        // defer stack of the frame under test hangs its one entry off.
        ctx.push_frame(
            mem::size_of::<StackFrameCommon>(),
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        ctx.stack_frame_mut::<StackFrameCommon>().frame_type = types().defer_frame.tid();
        let defer_stack =
            ctx.stack_pointer() as usize + mem::offset_of!(StackFrameCommon, defer_stack);
        unsafe {
            (defer_stack as *mut usize).write(entry as usize);
        }

        allocator.ptr().run_gc(&[&ctx]);
        assert!(allocator.ptr().contains(entry));
        assert!(allocator.ptr().contains(argument));
    }

    /// The conservative scan of a range walks it two words at a time while it
    /// can and then the word that is left over, stepping over the pairs of null
    /// words that keep nothing alive. A frame without a stack map is scanned
    /// that way, so this fills one with three leaves: the first two sit in a
    /// pair the scan takes at once and the third in the word it has to walk on
    /// its own.
    #[test]
    fn test_conservative_scan_of_a_frame_finds_a_pointer_in_every_word() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        // An odd number of words, so the last one is left over by the pairs.
        let words = 3;
        let frame_size = FRAME_LOCAL_OFFSET + words * mem::size_of::<usize>();
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        let frame = ctx.stack_pointer() as *mut u8;
        let mut leaves = Vec::new();
        for index in 0..words {
            let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
            unsafe {
                ptr::write_unaligned(
                    frame.add(FRAME_LOCAL_OFFSET + index * mem::size_of::<usize>()) as *mut usize,
                    leaf as usize,
                )
            };
            leaves.push(leaf);
        }

        allocator.ptr().run_gc(&[&ctx]);
        for leaf in leaves {
            assert!(allocator.ptr().contains(leaf));
        }
    }

    #[test]
    fn test_run_gc_reclaims_unreferenced_objects() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let kept = alloc_node(&allocator.ptr(), ptr::null_mut());
        let _root = register_root(&allocator.ptr(), types().node_ptr.tid(), &word(kept));
        let garbage = alloc_node(&allocator.ptr(), kept);
        let total_before = allocator.ptr().total_size();

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(kept));
        assert!(!allocator.ptr().contains(garbage));
        assert!(allocator.ptr().total_size() < total_before);
    }
}
