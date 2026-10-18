use std::collections::{BTreeMap, BTreeSet};
use std::ffi;
use std::mem;
use std::ptr;
use std::ptr::NonNull;
use std::rc::Rc;
use std::slice;

use allocator_api2::alloc::{AllocError, Allocator, Layout};

use crate::ClosureLayout;
use crate::FunctionObject;
use crate::defer_stack::DeferStackEntry;
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
use crate::type_id::GC_SLOT_MAP;
use crate::type_id::GC_SLOT_NO_POINTER;
use crate::type_id::GC_SLOT_POINTER;
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

const ARENA_SIZE: usize = 1 << 18;
const ARENA_PAGES: usize = ARENA_SIZE / PAGE_SIZE;
const WORD_SIZE: usize = mem::size_of::<usize>();
const BITMAP_SIZE: usize = ARENA_SIZE / WORD_SIZE / 8;
const POINTER_BITMAP_WORDS: usize = BITMAP_SIZE / WORD_SIZE;

fn bitmap_words_for_class(class: usize) -> usize {
    (ARENA_SIZE >> class).div_ceil(usize::BITS as usize)
}

fn bitmap_pages_for_class(class: usize) -> usize {
    (BITMAP_SIZE + 2 * bitmap_words_for_class(class) * WORD_SIZE).div_ceil(PAGE_SIZE)
}

const SLICE_HEADER_SIZE: usize = 3 * WORD_SIZE;
const STRING_SIZE: usize = 2 * WORD_SIZE;
pub(crate) const MAX_TOTAL_ALLOCATED_SIZE: usize = 1 << 20;
pub(crate) const LARGE_ALLOCATION_THRESHOLD: usize = 128 * 1024;

fn value_size(typ: TypeId) -> usize {
    match typ.kind() {
        TypeKind::None => 0,
        TypeKind::String => STRING_SIZE,
        TypeKind::Slice => SLICE_HEADER_SIZE,
        TypeKind::Interface => mem::size_of::<Interface>(),
        _ => typ.size(),
    }
}

fn size_class_of(size: usize) -> usize {
    size.max(1).next_power_of_two().max(8).trailing_zeros() as usize
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum ArenaKind {
    Object,
    Closure,
}

struct Arena {
    region: *mut (),
    bitmaps: *mut (),
    class: usize,
    allocation_bitmap: NonNull<[usize]>,
    mark_bitmap: NonNull<[usize]>,
    pointer_bitmap: NonNull<[usize; POINTER_BITMAP_WORDS]>,
    bitmap_pages: usize,
    allocation_pos: usize,
    kind: ArenaKind,
}

impl Arena {
    fn allocation_words(&self) -> &[usize] {
        unsafe { self.allocation_bitmap.as_ref() }
    }

    fn allocation_words_mut(&mut self) -> &mut [usize] {
        unsafe { self.allocation_bitmap.as_mut() }
    }

    fn mark_words(&self) -> &[usize] {
        unsafe { self.mark_bitmap.as_ref() }
    }

    fn mark_words_mut(&mut self) -> &mut [usize] {
        unsafe { self.mark_bitmap.as_mut() }
    }

    fn pointer_words_mut(&mut self) -> &mut [usize] {
        unsafe { self.pointer_bitmap.as_mut() }
    }
}

struct ObjectAllocatorInner {
    pager: Rc<Pager>,
    arenas: BTreeMap<usize, Arena>,
    allocated_objects: BTreeMap<usize, LargeAllocation>,
    large_closures: BTreeMap<usize, LargeAllocation>,
    allocated_channels: BTreeMap<usize, TypeId>,
    allocated_maps: BTreeSet<usize>,
    global_spans: Vec<GlobalSpan>,
    total_size: usize,
}

#[derive(Clone, Copy)]
struct Span {
    ptr: *mut (),
    size: usize,
}

#[derive(Clone, Copy)]
struct GlobalSpan {
    ptr: *mut (),
    size: usize,
    type_id: TypeId,
}

struct LargeAllocation {
    span: Span,
    marked: bool,
    type_id: TypeId,
}

fn get_bit(words: &[usize], index: usize) -> bool {
    let bits = usize::BITS as usize;
    (words[index / bits] >> (index % bits)) & 1 != 0
}

fn set_bit(words: &mut [usize], index: usize) {
    let bits = usize::BITS as usize;
    words[index / bits] |= 1usize << (index % bits);
}

fn clear_bit(words: &mut [usize], index: usize) {
    let bits = usize::BITS as usize;
    words[index / bits] &= !(1usize << (index % bits));
}

fn find_first_zero(words: &[usize], start: usize, count: usize) -> Option<usize> {
    (start..count).find(|&index| !get_bit(words, index))
}

fn popcount(words: &[usize], count: usize) -> usize {
    let bits = usize::BITS as usize;
    let full = count / bits;
    let rem = count % bits;
    let mut total: usize = words[..full]
        .iter()
        .map(|word| word.count_ones() as usize)
        .sum();
    if rem > 0 {
        total += (words[full] & ((1usize << rem) - 1)).count_ones() as usize;
    }
    total
}

extern "C" fn pointer_bitmap_visitor(
    offset: usize,
    size: usize,
    _typ: TypeId,
    kind: i32,
    arg: *mut ffi::c_void,
) {
    let bitmap = unsafe { slice::from_raw_parts_mut(arg as *mut usize, POINTER_BITMAP_WORDS) };
    let end = offset.saturating_add(size);
    match kind {
        GC_SLOT_POINTER | GC_SLOT_MAP => {
            let mut addr = offset;
            while addr + WORD_SIZE <= end {
                set_bit(bitmap, addr / WORD_SIZE);
                addr += WORD_SIZE;
            }
        }
        GC_SLOT_FUNCTION => {
            let mut addr = offset;
            while addr + mem::size_of::<FunctionObject>() <= end {
                set_bit(bitmap, addr / WORD_SIZE + 1);
                addr += mem::size_of::<FunctionObject>();
            }
        }
        GC_SLOT_SLICE => {
            let mut addr = offset;
            while addr + SLICE_HEADER_SIZE <= end {
                set_bit(bitmap, addr / WORD_SIZE);
                addr += SLICE_HEADER_SIZE;
            }
        }
        GC_SLOT_INTERFACE => {
            let mut addr = offset;
            while addr + mem::size_of::<Interface>() <= end {
                set_bit(bitmap, addr / WORD_SIZE);
                set_bit(bitmap, addr / WORD_SIZE + 1);
                addr += mem::size_of::<Interface>();
            }
        }
        GC_SLOT_STRING => {
            let mut addr = offset;
            while addr + STRING_SIZE <= end {
                set_bit(bitmap, addr / WORD_SIZE);
                addr += STRING_SIZE;
            }
        }
        GC_SLOT_DEFER_STACK | GC_SLOT_FREE_VARS => set_bit(bitmap, offset / WORD_SIZE),
        GC_SLOT_NO_POINTER => {}
        _ => panic!("unknown GC slot kind: {}", kind),
    }
}

fn build_slot_pointer_bitmap(
    bitmap: &mut [usize],
    slot_offset: usize,
    slot_size: usize,
    type_id: TypeId,
) {
    let first = slot_offset / WORD_SIZE;
    let last = first + slot_size / WORD_SIZE;
    let mut index = first;
    while index < last {
        clear_bit(bitmap, index);
        index += 1;
    }
    if !type_id.is_valid() {
        return;
    }
    match type_id.kind() {
        TypeKind::String
        | TypeKind::Slice
        | TypeKind::Pointer
        | TypeKind::Map
        | TypeKind::Interface => set_bit(bitmap, slot_offset / WORD_SIZE),
        TypeKind::Function => set_bit(bitmap, slot_offset / WORD_SIZE + 1),
        TypeKind::StructArray => {
            if let Some(get_member_offset_runs) = type_id.get_member_offset_runs() {
                get_member_offset_runs(
                    pointer_bitmap_visitor,
                    slot_offset,
                    bitmap.as_mut_ptr() as *mut ffi::c_void,
                );
            }
        }
        TypeKind::None | TypeKind::NoPointer => {}
    }
}

fn reserve_slot(
    arena: &mut Arena,
    base: usize,
    index: usize,
    type_id: TypeId,
    total_size: &mut usize,
) -> *mut () {
    let slot_size = 1usize << arena.class;
    set_bit(arena.allocation_words_mut(), index);
    let slot = (base + index * slot_size) as *mut u8;
    unsafe {
        ptr::write_bytes(slot, 0, slot_size);
    }
    build_slot_pointer_bitmap(
        arena.pointer_words_mut(),
        index * slot_size,
        slot_size,
        type_id,
    );
    arena.allocation_pos = index + 1;
    *total_size += slot_size;
    slot as *mut ()
}

fn slot_is_allocated(
    arenas: &BTreeMap<usize, Arena>,
    large: &BTreeMap<usize, LargeAllocation>,
    address: usize,
) -> bool {
    if large.contains_key(&address) {
        return true;
    }
    let arena_base = address & !(ARENA_SIZE - 1);
    if let Some(arena) = arenas.get(&arena_base) {
        let slot_size = 1usize << arena.class;
        let index = (address - arena_base) / slot_size;
        if index < (ARENA_SIZE >> arena.class) {
            return get_bit(arena.allocation_words(), index);
        }
    }
    false
}

impl ObjectAllocatorInner {
    fn new(pager: Rc<Pager>) -> Self {
        ObjectAllocatorInner {
            pager,
            arenas: BTreeMap::new(),
            allocated_objects: BTreeMap::new(),
            large_closures: BTreeMap::new(),
            allocated_channels: BTreeMap::new(),
            allocated_maps: BTreeSet::new(),
            global_spans: Vec::new(),
            total_size: 0,
        }
    }

    fn allocate(&mut self, size: usize, type_id: TypeId) -> *mut () {
        if size > LARGE_ALLOCATION_THRESHOLD {
            return self.allocate_pages(size, type_id);
        }
        self.allocate_slot(size_class_of(size), type_id, ArenaKind::Object)
    }

    fn allocate_channel(&mut self, size: usize, elem_type: TypeId) -> *mut () {
        let ptr = self.allocate(size, TypeId::new_invalid());
        if !ptr.is_null() {
            self.allocated_channels.insert(ptr as usize, elem_type);
        }
        ptr
    }

    fn allocate_closure(&mut self, size: usize) -> *mut () {
        if size > LARGE_ALLOCATION_THRESHOLD {
            let num_pages = size.div_ceil(PAGE_SIZE);
            let ptr = self.pager.allocate(num_pages, None);
            self.large_closures.insert(
                ptr as usize,
                LargeAllocation {
                    span: Span {
                        ptr,
                        size: PAGE_SIZE * num_pages,
                    },
                    marked: false,
                    type_id: TypeId::new_invalid(),
                },
            );
            return ptr;
        }
        self.allocate_slot(
            size_class_of(size),
            TypeId::new_invalid(),
            ArenaKind::Closure,
        )
    }

    fn allocate_map(&mut self, size: usize) -> *mut () {
        let ptr = self.allocate(size, TypeId::new_invalid());
        if !ptr.is_null() {
            self.allocated_maps.insert(ptr as usize);
        }
        ptr
    }

    fn allocate_slot(&mut self, class: usize, type_id: TypeId, kind: ArenaKind) -> *mut () {
        let slot_size = 1usize << class;
        if self.total_size.saturating_add(slot_size) > MAX_TOTAL_ALLOCATED_SIZE {
            return ptr::null_mut();
        }
        let slot_count = ARENA_SIZE >> class;
        let mut free_base = None;
        for (&base, arena) in self.arenas.iter() {
            if arena.class == class && arena.kind == kind {
                let pos = arena.allocation_pos;
                if find_first_zero(arena.allocation_words(), pos, slot_count).is_some() {
                    free_base = Some(base);
                    break;
                }
            }
        }
        if let Some(base) = free_base {
            let arena = self.arenas.get_mut(&base).unwrap();
            let pos = arena.allocation_pos;
            let index = find_first_zero(arena.allocation_words(), pos, slot_count).unwrap();
            return reserve_slot(arena, base, index, type_id, &mut self.total_size);
        }
        let base = self.create_arena(class, kind);
        let arena = self.arenas.get_mut(&base).unwrap();
        reserve_slot(arena, base, 0, type_id, &mut self.total_size)
    }

    fn create_arena(&mut self, class: usize, kind: ArenaKind) -> usize {
        let region = self.pager.allocate(ARENA_PAGES, Some(ARENA_SIZE));
        let base = region as usize;
        let alloc_words = bitmap_words_for_class(class);
        let bitmap_pages = bitmap_pages_for_class(class);
        let bitmaps_region = self.pager.allocate(bitmap_pages, None);
        let bitmaps = bitmaps_region as *mut usize;
        let words: &mut [usize] =
            unsafe { slice::from_raw_parts_mut(bitmaps, bitmap_pages * PAGE_SIZE / WORD_SIZE) };
        words.fill(0);
        let (pointer_bitmap, rest) = words.split_at_mut(POINTER_BITMAP_WORDS);
        let (allocation_bitmap, mark_and_padding) = rest.split_at_mut(alloc_words);
        let mark_bitmap = &mut mark_and_padding[..alloc_words];
        let pointer_bitmap: &mut [usize; POINTER_BITMAP_WORDS] = pointer_bitmap.try_into().unwrap();
        self.arenas.insert(
            base,
            Arena {
                region,
                bitmaps: bitmaps_region,
                class,
                allocation_bitmap: NonNull::from(allocation_bitmap),
                mark_bitmap: NonNull::from(mark_bitmap),
                pointer_bitmap: NonNull::from(pointer_bitmap),
                bitmap_pages,
                allocation_pos: 0,
                kind,
            },
        );
        base
    }

    fn allocate_pages(&mut self, size: usize, type_id: TypeId) -> *mut () {
        let num_pages = size.div_ceil(PAGE_SIZE);
        let ptr = self.pager.allocate(num_pages, None);
        self.allocated_objects.insert(
            ptr as usize,
            LargeAllocation {
                span: Span {
                    ptr,
                    size: PAGE_SIZE * num_pages,
                },
                marked: false,
                type_id,
            },
        );
        ptr
    }

    fn free_large(&mut self, object: &LargeAllocation) {
        self.pager
            .deallocate(object.span.ptr, object.span.size / PAGE_SIZE);
    }

    fn free_arena(&mut self, arena: &Arena) {
        self.pager.deallocate(arena.bitmaps, arena.bitmap_pages);
        self.pager.deallocate(arena.region, ARENA_PAGES);
    }

    fn sweep(&mut self) {
        let objects = mem::take(&mut self.allocated_objects);
        self.allocated_objects = objects
            .into_iter()
            .filter_map(|(address, object)| {
                if object.marked {
                    Some((address, object))
                } else {
                    self.free_large(&object);
                    None
                }
            })
            .collect();
        let closures = mem::take(&mut self.large_closures);
        self.large_closures = closures
            .into_iter()
            .filter_map(|(address, object)| {
                if object.marked {
                    Some((address, object))
                } else {
                    self.free_large(&object);
                    None
                }
            })
            .collect();

        let mut freed = Vec::new();
        for (&base, arena) in self.arenas.iter_mut() {
            let slot_size = 1usize << arena.class;
            let slot_count = ARENA_SIZE >> arena.class;
            let alloc_count = popcount(arena.allocation_words(), slot_count);
            let mark_count = popcount(arena.mark_words(), slot_count);
            if mark_count == 0 {
                freed.push(base);
            } else {
                mem::swap(&mut arena.allocation_bitmap, &mut arena.mark_bitmap);
                arena.allocation_pos = 0;
                self.total_size = self
                    .total_size
                    .checked_sub((alloc_count - mark_count) * slot_size)
                    .expect("total allocated size underflow while sweeping an arena");
            }
        }
        for base in freed {
            let arena = self.arenas.remove(&base).unwrap();
            let slot_size = 1usize << arena.class;
            let slot_count = ARENA_SIZE >> arena.class;
            let alloc_count = popcount(arena.allocation_words(), slot_count);
            self.total_size = self
                .total_size
                .checked_sub(alloc_count * slot_size)
                .expect("total allocated size underflow while freeing an arena");
            self.free_arena(&arena);
        }

        let arenas = &self.arenas;
        let large = &self.allocated_objects;
        self.allocated_channels
            .retain(|address, _| slot_is_allocated(arenas, large, *address));
        self.allocated_maps
            .retain(|address| slot_is_allocated(arenas, large, *address));
    }

    fn register_global_object(&mut self, address: *mut (), size: usize, type_id: TypeId) {
        self.global_spans.push(GlobalSpan {
            ptr: address,
            size,
            type_id,
        });
    }

    fn free_all_allocated_objects(&mut self) {
        for arena in self.arenas.values_mut() {
            arena.mark_words_mut().fill(0);
        }
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        for object in self.large_closures.values_mut() {
            object.marked = false;
        }
        self.sweep();
        assert!(
            self.total_size == 0,
            "total allocated size {} did not reach zero after freeing all objects",
            self.total_size
        );
    }

    fn class_has_no_free_slot(&self, size: usize, kind: ArenaKind) -> bool {
        if size > LARGE_ALLOCATION_THRESHOLD {
            return false;
        }
        let class = size_class_of(size);
        let slot_count = ARENA_SIZE >> class;
        let mut saw_arena = false;
        for arena in self.arenas.values() {
            if arena.class == class && arena.kind == kind {
                saw_arena = true;
                let pos = arena.allocation_pos;
                if find_first_zero(arena.allocation_words(), pos, slot_count).is_some() {
                    return false;
                }
            }
        }
        saw_arena
    }

    pub(crate) fn run_gc(&mut self, contexts: &[&LightWeightThreadContext]) {
        for arena in self.arenas.values_mut() {
            arena.mark_words_mut().fill(0);
        }
        for object in self.allocated_objects.values_mut() {
            object.marked = false;
        }
        for object in self.large_closures.values_mut() {
            object.marked = false;
        }
        let global_spans = mem::take(&mut self.global_spans);
        for span in &global_spans {
            assert!(
                span.type_id.is_valid(),
                "global object of size {} was registered without a type descriptor",
                span.size
            );
            self.mark_value(span.type_id, span.ptr as usize);
        }
        self.global_spans = global_spans;
        for context in contexts {
            self.mark_stack(context);
        }
        self.sweep();
    }

    fn mark_stack(&mut self, context: &LightWeightThreadContext) {
        for frame in context.stack_frames() {
            if let Some(get_member_offset_runs) = frame.frame_type.get_member_offset_runs() {
                let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
                get_member_offset_runs(mark_typed_visitor, frame.address, inner);
            }
        }
        for root in context.gc_roots() {
            self.mark_typed_offset(root.address, root.size, root.type_id, root.kind);
        }
    }

    fn mark_raw_word(&mut self, address: usize) {
        let word = unsafe { ptr::read_unaligned(address as *const usize) };
        self.mark_word(word);
    }

    fn mark_word(&mut self, word: usize) {
        if word == 0 {
            return;
        }
        if let Some(object_address) = self.resolve_any(word) {
            self.mark_object(object_address);
        }
    }

    fn resolve_any(&self, address: usize) -> Option<usize> {
        self.containing_object(address)
            .or_else(|| self.containing_closure(address))
    }

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
        if let Some(object_address) = self.resolve_any(word) {
            self.mark_object(object_address);
            if pointed_to != TypeId::new_invalid() {
                self.mark_value_at(word, pointed_to);
            }
        }
    }

    fn mark_value_at(&mut self, address: usize, typ: TypeId) {
        let size = value_size(typ);
        if size == 0 {
            return;
        }
        if let Some(end) = self.allocation_end(address)
            && address + size <= end
        {
            self.mark_value(typ, address);
        }
    }

    fn allocation_end(&self, address: usize) -> Option<usize> {
        let arena_base = address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get(&arena_base) {
            let slot_size = 1usize << arena.class;
            let index = (address - arena_base) / slot_size;
            if index < (ARENA_SIZE >> arena.class) && get_bit(arena.allocation_words(), index) {
                return Some(arena_base + (index + 1) * slot_size);
            }
            return None;
        }
        if let Some((&base, object)) = self.allocated_objects.range(..=address).next_back()
            && address < base + object.span.size
        {
            return Some(base + object.span.size);
        }
        if let Some((&base, object)) = self.large_closures.range(..=address).next_back()
            && address < base + object.span.size
        {
            return Some(base + object.span.size);
        }
        None
    }

    fn mark_slice_header(&mut self, addr: usize, elem_type: TypeId) {
        let word_size = mem::size_of::<usize>();
        let data = unsafe { ptr::read_unaligned(addr as *const usize) };
        let len = unsafe { ptr::read_unaligned((addr + word_size) as *const usize) };
        if data == 0 || len == 0 {
            return;
        }
        let Some(object_address) = self.containing_object(data) else {
            return;
        };
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
        if let Some(allocation_end) = self.allocation_end(object_address) {
            end = end.min(allocation_end);
        }
        if end <= data {
            return;
        }
        self.scan_value_range(elem_type, data, end);
    }

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
                while addr + mem::size_of::<FunctionObject>() <= end {
                    self.mark_function_field(addr);
                    addr += mem::size_of::<FunctionObject>();
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
        let Some(object_address) = self.resolve_any(receiver) else {
            return;
        };
        self.mark_object(object_address);
        let type_word = unsafe { ptr::read_unaligned((addr + word_size) as *const usize) };
        let concrete = TypeId::from_raw(type_word);
        if concrete.is_valid() {
            self.mark_value_at(receiver, concrete);
        }
    }

    fn mark_function_field(&mut self, addr: usize) {
        let env = unsafe {
            ptr::read_unaligned((addr + mem::offset_of!(FunctionObject, 1)) as *const usize)
        };
        if env != 0
            && let Some(closure_address) = self.containing_closure(env)
        {
            self.mark_object(closure_address);
        }
    }

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
        }
    }

    fn mark_slot_pointer_bitmap(
        &mut self,
        pointer_bitmap: NonNull<[usize; POINTER_BITMAP_WORDS]>,
        index: usize,
        slot_size: usize,
        slot_base: usize,
    ) {
        let pointer_bitmap = unsafe { pointer_bitmap.as_ref() };
        let words = slot_size / WORD_SIZE;
        let first = index * words;
        let mut k = 0;
        while k < words {
            if get_bit(pointer_bitmap, first + k) {
                self.mark_raw_word(slot_base + k * WORD_SIZE);
            }
            k += 1;
        }
    }

    fn mark_object(&mut self, object_address: usize) {
        let arena_base = object_address & !(ARENA_SIZE - 1);
        let arena_info = self.arenas.get(&arena_base).map(|arena| {
            (
                arena.class,
                arena.kind,
                arena.pointer_bitmap,
                arena.mark_bitmap,
            )
        });
        if let Some((class, kind, pointer_bitmap, mut mark_bitmap)) = arena_info {
            let slot_size = 1usize << class;
            let slot_count = ARENA_SIZE >> class;
            let index = (object_address - arena_base) / slot_size;
            if index >= slot_count {
                return;
            }
            {
                let mark_bitmap = unsafe { mark_bitmap.as_mut() };
                if get_bit(mark_bitmap, index) {
                    return;
                }
                set_bit(mark_bitmap, index);
            }
            let slot_base = arena_base + index * slot_size;
            if self.allocated_channels.contains_key(&slot_base) {
                self.mark_channel_scan(slot_base);
                return;
            }
            if self.allocated_maps.contains(&slot_base) {
                self.mark_map_scan(slot_base);
                return;
            }
            if kind == ArenaKind::Closure {
                self.mark_closure_scan(slot_base);
                return;
            }
            self.mark_slot_pointer_bitmap(pointer_bitmap, index, slot_size, slot_base);
            return;
        }
        if let Some(object) = self.large_closures.get_mut(&object_address) {
            if object.marked {
                return;
            }
            object.marked = true;
            self.mark_closure_scan(object_address);
            return;
        }
        if let Some(object) = self.allocated_objects.get_mut(&object_address) {
            if object.marked {
                return;
            }
            object.marked = true;
            let type_id = object.type_id;
            if self.allocated_channels.contains_key(&object_address) {
                self.mark_channel_scan(object_address);
                return;
            }
            if self.allocated_maps.contains(&object_address) {
                self.mark_map_scan(object_address);
                return;
            }
            if type_id.is_valid() {
                self.mark_value(type_id, object_address);
            }
        }
    }

    fn mark_object_without_interior(&mut self, object_address: usize) {
        let arena_base = object_address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get_mut(&arena_base) {
            let slot_size = 1usize << arena.class;
            let index = (object_address - arena_base) / slot_size;
            if index < (ARENA_SIZE >> arena.class) {
                set_bit(arena.mark_words_mut(), index);
            }
            return;
        }
        if let Some(object) = self.allocated_objects.get_mut(&object_address) {
            object.marked = true;
        } else if let Some(object) = self.large_closures.get_mut(&object_address) {
            object.marked = true;
        }
    }

    fn mark_closure_scan(&mut self, base: usize) {
        let capture_type_word = unsafe {
            ptr::read_unaligned(
                (base + mem::offset_of!(ClosureLayout, capture_type)) as *const usize,
            )
        };
        let capture_start = base + mem::size_of::<ClosureLayout>();
        let capture_type = TypeId::from_raw(capture_type_word);
        if let Some(get_member_offset_runs) = capture_type.get_member_offset_runs() {
            let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
            get_member_offset_runs(mark_typed_visitor, capture_start, inner);
        }
    }

    fn mark_channel_scan(&mut self, base: usize) {
        let elem_type = self
            .allocated_channels
            .get(&base)
            .copied()
            .unwrap_or(TypeId::new_invalid());
        let channel = unsafe { &*(base as *const ChannelObject) };
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
        let map = unsafe { &*(base as *const MapObject) };
        if let Some(address) = map.table_address()
            && let Some(object_address) = self.containing_object(address)
        {
            self.mark_object(object_address);
        }
        map.gc_keep(|key_type, key_address, value_type, value_address| {
            self.mark_box_with_type(key_address, key_type);
            self.mark_box_with_type(value_address, value_type);
        });
    }

    fn mark_box_with_type(&mut self, address: usize, typ: TypeId) {
        if let Some(object_address) = self.containing_object(address) {
            self.mark_object(object_address);
            if typ != TypeId::new_invalid() {
                self.mark_value_at(address, typ);
            }
        }
    }

    fn containing_object(&self, address: usize) -> Option<usize> {
        let arena_base = address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get(&arena_base) {
            if arena.kind == ArenaKind::Object {
                let slot_size = 1usize << arena.class;
                let index = (address - arena_base) / slot_size;
                if index < (ARENA_SIZE >> arena.class) && get_bit(arena.allocation_words(), index) {
                    return Some(arena_base + index * slot_size);
                }
            }
            return None;
        }
        if let Some((&base, object)) = self.allocated_objects.range(..=address).next_back()
            && address < base + object.span.size
        {
            return Some(base);
        }
        None
    }

    fn containing_closure(&self, address: usize) -> Option<usize> {
        let arena_base = address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get(&arena_base) {
            if arena.kind == ArenaKind::Closure {
                let slot_size = 1usize << arena.class;
                let index = (address - arena_base) / slot_size;
                if index < (ARENA_SIZE >> arena.class) && get_bit(arena.allocation_words(), index) {
                    return Some(arena_base + index * slot_size);
                }
            }
            return None;
        }
        if let Some((&base, object)) = self.large_closures.range(..=address).next_back()
            && address < base + object.span.size
        {
            return Some(base);
        }
        None
    }

    #[cfg(test)]
    fn contains_object(&self, address: usize) -> bool {
        if self.allocated_objects.contains_key(&address) {
            return true;
        }
        let arena_base = address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get(&arena_base)
            && arena.kind == ArenaKind::Object
        {
            let slot_size = 1usize << arena.class;
            if (address - arena_base).is_multiple_of(slot_size) {
                let index = (address - arena_base) / slot_size;
                return get_bit(arena.allocation_words(), index);
            }
        }
        false
    }

    #[cfg(test)]
    fn contains_closure(&self, address: usize) -> bool {
        if self.large_closures.contains_key(&address) {
            return true;
        }
        let arena_base = address & !(ARENA_SIZE - 1);
        if let Some(arena) = self.arenas.get(&arena_base)
            && arena.kind == ArenaKind::Closure
        {
            let slot_size = 1usize << arena.class;
            if (address - arena_base).is_multiple_of(slot_size) {
                let index = (address - arena_base) / slot_size;
                return get_bit(arena.allocation_words(), index);
            }
        }
        false
    }
}

impl ObjectAllocatorInner {
    fn mark_defer_stack_slot(&mut self, offset: usize) {
        let mut word = unsafe { ptr::read_unaligned(offset as *const usize) };
        while word != 0 {
            let Some(entry_address) = self.resolve_any(word) else {
                break;
            };
            let (func_address, args_address, args_type, next) = {
                let entry = unsafe { &*(entry_address as *const DeferStackEntry) };
                (
                    entry.func_address(),
                    entry.args_words_address(),
                    entry.args_type(),
                    entry.next_address(),
                )
            };
            self.mark_object(entry_address);
            self.mark_function_field(func_address);
            if let Some(get_member_offset_runs) = args_type.get_member_offset_runs() {
                let inner = self as *mut ObjectAllocatorInner as *mut ffi::c_void;
                get_member_offset_runs(mark_typed_visitor, args_address, inner);
            }
            word = next;
        }
    }

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
                while address + mem::size_of::<FunctionObject>() <= end {
                    self.mark_function_field(address);
                    address += mem::size_of::<FunctionObject>();
                }
            }
            GC_SLOT_DEFER_STACK => self.mark_defer_stack_slot(offset),
            GC_SLOT_FREE_VARS => {
                let word = unsafe { ptr::read_unaligned(offset as *const usize) };
                if word != 0
                    && let Some(closure_address) = self.containing_closure(word)
                {
                    self.mark_object(closure_address);
                }
            }
            GC_SLOT_MAP => {
                let mut address = offset;
                let end = offset.saturating_add(size);
                while address + word_size <= end {
                    self.mark_map_slot(address);
                    address += word_size;
                }
            }
            GC_SLOT_NO_POINTER => {}
            _ => panic!("unknown GC slot kind: {}", kind),
        }
    }
}

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

const _: TypeOffsetVisitor = mark_typed_visitor;

#[derive(Clone)]
pub(crate) struct ObjectAllocatorPtr(*mut ObjectAllocatorInner);

impl ObjectAllocatorPtr {
    pub(crate) fn allocate(&self, size: usize, type_id: TypeId) -> *mut () {
        unsafe { &mut *self.0 }.allocate(size, type_id)
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

    pub(crate) fn class_has_no_free_slot(&self, size: usize, closure: bool) -> bool {
        let kind = if closure {
            ArenaKind::Closure
        } else {
            ArenaKind::Object
        };
        unsafe { &*self.0 }.class_has_no_free_slot(size, kind)
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
        unsafe { &*self.0 }.contains_object(ptr as usize)
    }

    #[cfg(test)]
    pub(crate) fn contains_closure(&self, ptr: *mut ()) -> bool {
        unsafe { &*self.0 }.contains_closure(ptr as usize)
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
        let ptr = if layout.align() <= WORD_SIZE {
            let ptr = self.allocate(layout.size(), TypeId::new_invalid());
            if ptr.is_null() {
                return Err(AllocError);
            }
            ptr
        } else {
            let total = layout.size() + layout.align();
            let base = self.allocate(total, TypeId::new_invalid());
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

    extern "C" fn mixed_generator(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void) {
        let node = *NODE_TID.get().unwrap();
        visit(base, mem::size_of::<usize>(), node, GC_SLOT_POINTER, arg);
        visit(base + 8, SLICE_HEADER_SIZE, node, GC_SLOT_SLICE, arg);
        visit(
            base + 32,
            STRING_SIZE,
            TypeId::new_invalid(),
            GC_SLOT_STRING,
            arg,
        );
        visit(
            base + 48,
            2 * mem::size_of::<usize>(),
            TypeId::new_invalid(),
            GC_SLOT_INTERFACE,
            arg,
        );
        visit(
            base + 64,
            mem::size_of::<FunctionObject>(),
            TypeId::new_invalid(),
            GC_SLOT_FUNCTION,
            arg,
        );
        visit(
            base + 80,
            mem::size_of::<usize>(),
            TypeId::new_invalid(),
            GC_SLOT_POINTER,
            arg,
        );
    }

    /// A struct whose only pointer-bearing member is a map value.
    extern "C" fn map_slot_generator(visit: TypeOffsetVisitor, base: usize, arg: *mut ffi::c_void) {
        visit(
            base,
            mem::size_of::<usize>(),
            TypeId::new_invalid(),
            GC_SLOT_MAP,
            arg,
        );
    }

    /// A type with pointer words in two different bitmap words: one at the
    /// object base and one past the first 512 bytes.
    extern "C" fn far_pointer_generator(
        visit: TypeOffsetVisitor,
        base: usize,
        arg: *mut ffi::c_void,
    ) {
        let node = *NODE_TID.get().unwrap();
        visit(base, mem::size_of::<usize>(), node, GC_SLOT_POINTER, arg);
        visit(
            base + 520,
            mem::size_of::<usize>(),
            node,
            GC_SLOT_POINTER,
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
        /// struct { m map[...]... }, one word.
        map_slot: FakeTypeInfo,
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
            let map_slot = FakeTypeInfo::new_with(
                TypeKind::StructArray,
                TypeId::new_invalid(),
                Some(map_slot_generator),
                mem::size_of::<usize>(),
            );
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
                map_slot,
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
        let node = allocator.allocate(16, TypeId::new_invalid());
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
        let ptr = allocator.ptr().allocate(size, TypeId::new_invalid());
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
        let ptr = allocator.ptr().allocate(size, TypeId::new_invalid());
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
            let ptr = allocator.ptr().allocate(size, TypeId::new_invalid());
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
    fn test_large_allocation_is_traced_by_its_type() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let typ = FakeTypeInfo::new_with(
            TypeKind::StructArray,
            TypeId::new_invalid(),
            Some(node_generator),
            LARGE_ALLOCATION_THRESHOLD + 1,
        );
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let region = allocator
            .ptr()
            .allocate(LARGE_ALLOCATION_THRESHOLD + 1, typ.tid());
        // The pointer to the leaf sits at the first word of the region, which
        // the node generator reports, so the page-backed allocation is traced
        // through its descriptor like a heap one.
        unsafe { (region as *mut usize).write(leaf as usize) };
        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(region));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(region));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_allocate_rounds_size_up_and_registers_object() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let ptr = allocator.ptr().allocate(1, TypeId::new_invalid());
        assert!(!ptr.is_null());
        assert!(allocator.ptr().contains(ptr));
        assert!(!allocator.ptr().contains_closure(ptr));
    }

    #[test]
    fn test_arena_bitmaps_share_one_pager_allocation_and_are_freed_together() {
        let pager = Rc::new(Pager::new());
        let allocator = ObjectAllocator::new(pager.clone());
        let size = 8;
        let ptr = allocator.ptr().allocate(size, TypeId::new_invalid());
        assert!(!ptr.is_null());
        assert_eq!(
            pager.allocated_pages(),
            ARENA_PAGES + bitmap_pages_for_class(size_class_of(size))
        );
        allocator.ptr().run_gc(&[]);
        assert_eq!(pager.allocated_pages(), 0);
    }

    #[test]
    fn test_allocate_returns_null_when_heap_is_exhausted() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        while !allocator
            .ptr()
            .allocate(65536, TypeId::new_invalid())
            .is_null()
        {}
        assert!(
            allocator
                .ptr()
                .allocate(65536, TypeId::new_invalid())
                .is_null()
        );
    }

    #[test]
    fn test_allocate_closure_is_registered_apart_from_ordinary_objects() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let closure = allocator.ptr().allocate_closure(32);
        assert!(!closure.is_null());
        assert!(!allocator.ptr().contains(closure));
        assert!(allocator.ptr().contains_closure(closure));

        let ordinary = allocator.ptr().allocate(32, TypeId::new_invalid());
        assert!(allocator.ptr().contains(ordinary));
        assert!(!allocator.ptr().contains_closure(ordinary));
    }

    #[test]
    fn test_untyped_frame_without_a_root_is_not_traced() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let target = alloc_node(&allocator.ptr(), ptr::null_mut());
        let frame_size = FRAME_LOCAL_OFFSET + mem::size_of::<usize>();
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        unsafe {
            (ctx.stack_pointer() as *mut usize).write(target as usize);
        }

        // A frame the runtime pushed is not read at all: only a registered
        // pointer word roots an object.
        allocator.ptr().run_gc(&[&ctx]);
        assert!(!allocator.ptr().contains(target));
    }

    #[test]
    fn test_frame_pointer_root_keeps_the_object() {
        let (mut ctx, _gc) = create_ctx();
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let frame_size = FRAME_LOCAL_OFFSET + mem::size_of::<usize>();
        ctx.push_frame(
            frame_size,
            ctx.stack_pointer(),
            None,
            &[],
            FunctionObject::new_null(),
        );
        let frame_address = ctx.stack_pointer() as usize;
        let target = alloc_node(&allocator.ptr(), ptr::null_mut());
        unsafe {
            (ctx.stack_pointer() as *mut usize).write(target as usize);
        }
        ctx.push_gc_root(
            frame_address,
            frame_address,
            mem::size_of::<usize>(),
            TypeId::new_invalid(),
            GC_SLOT_POINTER,
        );

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
    #[should_panic(expected = "without a type descriptor")]
    fn test_untyped_global_root_is_rejected() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let target = alloc_node(&allocator.ptr(), ptr::null_mut());
        let _root = register_root(&allocator.ptr(), TypeId::new_invalid(), &word(target));

        allocator.ptr().run_gc(&[]);
    }

    #[test]
    fn test_string_global_keeps_its_buffer() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let buffer = allocator.ptr().allocate(8, TypeId::new_invalid());
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
        let buffer = allocator
            .ptr()
            .allocate(cap * mem::size_of::<usize>(), TypeId::new_invalid());
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
    fn test_pointer_bitmap_marks_every_pointer_bearing_slot() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let typ = FakeTypeInfo::new_with(
            TypeKind::StructArray,
            TypeId::new_invalid(),
            Some(mixed_generator),
            88,
        );
        let object = allocator.ptr().allocate(88, typ.tid());
        let first = alloc_node(&allocator.ptr(), ptr::null_mut());
        let buffer = allocator.ptr().allocate(8, TypeId::new_invalid());
        let data = allocator.ptr().allocate(8, TypeId::new_invalid());
        let receiver = alloc_node(&allocator.ptr(), ptr::null_mut());
        let closure = allocator.ptr().allocate_closure(32);
        let raw = alloc_node(&allocator.ptr(), ptr::null_mut());
        unsafe {
            let words = object as *mut usize;
            words.add(0).write(first as usize);
            words.add(1).write(buffer as usize);
            words.add(2).write(1);
            words.add(3).write(1);
            words.add(4).write(data as usize);
            words.add(5).write(1);
            words.add(6).write(receiver as usize);
            words.add(7).write(0);
            words.add(8).write(0);
            words.add(9).write(closure as usize);
            words.add(10).write(raw as usize);
        }
        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(object));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(object));
        assert!(allocator.ptr().contains(first));
        assert!(allocator.ptr().contains(buffer));
        assert!(allocator.ptr().contains(data));
        assert!(allocator.ptr().contains(receiver));
        assert!(allocator.ptr().contains_closure(closure));
        assert!(allocator.ptr().contains(raw));
    }

    #[test]
    fn test_pointer_bitmap_handles_objects_over_512_bytes() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let typ = FakeTypeInfo::new_with(
            TypeKind::StructArray,
            TypeId::new_invalid(),
            Some(far_pointer_generator),
            1024,
        );
        let object = allocator.ptr().allocate(1024, typ.tid());
        let low = alloc_node(&allocator.ptr(), ptr::null_mut());
        let high = alloc_node(&allocator.ptr(), ptr::null_mut());
        unsafe {
            let words = object as *mut usize;
            words.add(0).write(low as usize);
            words.add(65).write(high as usize);
        }
        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(object));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(object));
        assert!(allocator.ptr().contains(low));
        assert!(allocator.ptr().contains(high));
    }

    #[test]
    fn test_object_larger_than_the_bitmap_is_traced_by_type() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let typ = FakeTypeInfo::new_with(
            TypeKind::StructArray,
            TypeId::new_invalid(),
            Some(node_generator),
            LARGE_ALLOCATION_THRESHOLD + 1,
        );
        let object = allocator
            .ptr()
            .allocate(LARGE_ALLOCATION_THRESHOLD + 1, typ.tid());
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        unsafe { (object as *mut usize).write(leaf as usize) };
        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(object));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(object));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_slice_of_struct_elements_traces_their_members() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let buffer = allocator.ptr().allocate(16, TypeId::new_invalid());
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
        let box_ptr = allocator.ptr().allocate(8, TypeId::new_invalid());
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
        let mut root = [0u8; 16];
        root[..8].copy_from_slice(&raw_word(function.func_address()));
        root[8..16].copy_from_slice(&raw_word(function.env_address()));
        let _root = register_root(&allocator.ptr(), types().function.tid(), &root);

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
    fn test_map_slot_marks_the_map_and_its_entries() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let map_ptr = allocator.ptr().allocate_map(mem::size_of::<MapObject>()) as *mut MapObject;
        MapObject::construct_in(map_ptr, map_key_type(), types().node.tid(), allocator.ptr());

        let mut key = [0u64; 2];
        key[0] = 11;
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let value = alloc_node(&allocator.ptr(), leaf);
        unsafe {
            (&mut *map_ptr).set(ObjectPtr(key.as_mut_ptr() as *mut ()), ObjectPtr(value));
        }

        let mut slot = [0u8; 8];
        slot.copy_from_slice(&word(map_ptr as *mut ()));
        let _root = register_root(&allocator.ptr(), types().map_slot.tid(), &slot);

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(map_ptr as *mut ()));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_map_slot_pointer_bitmap_keeps_the_map() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let object = allocator
            .ptr()
            .allocate(mem::size_of::<usize>(), types().map_slot.tid());
        let map_ptr = allocator.ptr().allocate_map(mem::size_of::<MapObject>()) as *mut MapObject;
        MapObject::construct_in(map_ptr, map_key_type(), types().node.tid(), allocator.ptr());
        let mut key = [0u64; 2];
        key[0] = 13;
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let value = alloc_node(&allocator.ptr(), leaf);
        unsafe {
            (object as *mut usize).write(map_ptr as usize);
            (&mut *map_ptr).set(ObjectPtr(key.as_mut_ptr() as *mut ()), ObjectPtr(value));
        }

        let _root = register_root(&allocator.ptr(), types().opaque_ptr.tid(), &word(object));

        allocator.ptr().run_gc(&[]);
        assert!(allocator.ptr().contains(object));
        assert!(allocator.ptr().contains(map_ptr as *mut ()));
        assert!(allocator.ptr().contains(leaf));
    }

    #[test]
    fn test_sweep_prunes_channel_and_map_registrations() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let channel = allocator
            .ptr()
            .allocate(mem::size_of::<ChannelObject>(), TypeId::new_invalid());
        allocator
            .ptr()
            .allocate_channel(mem::size_of::<ChannelObject>(), types().node.tid());
        let map = allocator
            .ptr()
            .allocate(mem::size_of::<MapObject>(), TypeId::new_invalid());
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
        let region = allocator.allocate(LARGE_ALLOCATION_THRESHOLD + 1, TypeId::new_invalid());
        unsafe { (region as *mut usize).write(0) };
        unsafe { (region as *mut usize).add(offset) }
    }

    #[test]
    fn test_pointer_to_interior_address_traces_the_value_there() {
        let allocator = ObjectAllocator::new(Rc::new(Pager::new()));
        let leaf = alloc_node(&allocator.ptr(), ptr::null_mut());
        let buffer = allocator
            .ptr()
            .allocate(mem::size_of::<usize>(), TypeId::new_invalid());
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
        // The argument words of the deferred call, as a WordChunk: a count
        // followed by the words themselves. The entry owns the copy and points
        // its argument enumerator at the first of them.
        let args = [1usize, argument as usize];
        let entry = allocator
            .ptr()
            .allocate(mem::size_of::<DeferStackEntry>(), TypeId::new_invalid())
            as *mut DeferStackEntry;
        unsafe {
            ptr::write(
                entry,
                DeferStackEntry::new(
                    FunctionObject::new_null(),
                    0,
                    NonNull::new(args.as_ptr() as *mut crate::word_chunk::WordChunk).unwrap(),
                    TypeId::new_invalid(),
                    types().node.tid(),
                ),
            );
        };

        // The frame's stack map lists its defer stack, and the stack hangs the
        // one entry off, so the walk traces the entry and its argument.
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
        assert!(allocator.ptr().contains(entry as *mut ()));
        assert!(allocator.ptr().contains(argument));
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
