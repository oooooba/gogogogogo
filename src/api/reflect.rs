use std::ptr;

use crate::FunctionObject;
use crate::LightWeightThreadContext;
use crate::StackFrameCommon;
use crate::object::string::StringObject;
use crate::type_id::TypeId;

// reflect.Kind values, matching internal/abi.Kind.
const KIND_INVALID: i32 = 0;
const KIND_BOOL: i32 = 1;
const KIND_INT: i32 = 2;
const KIND_INT8: i32 = 3;
const KIND_INT16: i32 = 4;
const KIND_INT32: i32 = 5;
const KIND_INT64: i32 = 6;
const KIND_UINT: i32 = 7;
const KIND_UINT8: i32 = 8;
const KIND_UINT16: i32 = 9;
const KIND_UINT32: i32 = 10;
const KIND_UINT64: i32 = 11;
const KIND_UINTPTR: i32 = 12;
const KIND_FLOAT32: i32 = 13;
const KIND_FLOAT64: i32 = 14;
const KIND_COMPLEX64: i32 = 15;
const KIND_COMPLEX128: i32 = 16;
const KIND_ARRAY: i32 = 17;
const KIND_CHAN: i32 = 18;
const KIND_FUNC: i32 = 19;
const KIND_INTERFACE: i32 = 20;
const KIND_MAP: i32 = 21;
const KIND_POINTER: i32 = 22;
const KIND_SLICE: i32 = 23;
const KIND_STRING: i32 = 24;
const KIND_STRUCT: i32 = 25;
const KIND_UNSAFEPOINTER: i32 = 26;

struct TypeMapping {
    kind: i32,
    go_name: &'static str,
}

fn mapping_for_basic(cname: &str) -> Option<TypeMapping> {
    // The runtime stores each boxed value's *C representation* type name in its
    // TypeInfo (e.g. "IntObject", "StringObject"), not the Go source type name.
    // Map those to the reflect.Kind and Go type name fmt needs.
    Some(TypeMapping {
        kind: match cname {
            "BoolObject" => KIND_BOOL,
            "IntObject" => KIND_INT,
            "Int8Object" => KIND_INT8,
            "Int16Object" => KIND_INT16,
            "Int32Object" => KIND_INT32,
            "Int64Object" => KIND_INT64,
            "UintObject" => KIND_UINT,
            "Uint8Object" => KIND_UINT8,
            "Uint16Object" => KIND_UINT16,
            "Uint32Object" => KIND_UINT32,
            "Uint64Object" => KIND_UINT64,
            "UintptrObject" => KIND_UINTPTR,
            "Float32Object" => KIND_FLOAT32,
            "Float64Object" => KIND_FLOAT64,
            "Complex64Object" => KIND_COMPLEX64,
            "Complex128Object" => KIND_COMPLEX128,
            "StringObject" => KIND_STRING,
            "UnsafePointerObject" => KIND_UNSAFEPOINTER,
            _ => return None,
        },
        go_name: match cname {
            "BoolObject" => "bool",
            "IntObject" => "int",
            "Int8Object" => "int8",
            "Int16Object" => "int16",
            "Int32Object" => "int32",
            "Int64Object" => "int64",
            "UintObject" => "uint",
            "Uint8Object" => "uint8",
            "Uint16Object" => "uint16",
            "Uint32Object" => "uint32",
            "Uint64Object" => "uint64",
            "UintptrObject" => "uintptr",
            "Float32Object" => "float32",
            "Float64Object" => "float64",
            "Complex64Object" => "complex64",
            "Complex128Object" => "complex128",
            "StringObject" => "string",
            "UnsafePointerObject" => "unsafe.Pointer",
            _ => "",
        },
    })
}

/// Best-effort mapping of a C representation type name to a reflect.Kind and a
/// Go type name. Basic types are exact; composite/named types are inferred from
/// the "Named<GoName$underlying>", "Slice<T>", "Pointer<T>" etc. shapes, or
/// fall back to (Struct, "") when unrecognized.
fn mapping_for_name(cname: &str) -> (i32, String) {
    if let Some(m) = mapping_for_basic(cname) {
        return (m.kind, m.go_name.to_string());
    }

    if let Some(inner) = cname.strip_prefix("Named<") {
        let mut split = inner.splitn(2, '$');
        let go_name = split.next().unwrap_or("").to_string();
        let underlying = split.next().unwrap_or("");
        let (kind, _) = mapping_for_name(underlying);
        return (kind, go_name);
    }
    if cname.starts_with("Slice<") {
        return (KIND_SLICE, format!("[]{}", inner_of(cname, "Slice<")));
    }
    if cname.starts_with("Pointer<") {
        return (KIND_POINTER, format!("*{}", inner_of(cname, "Pointer<")));
    }
    if cname.starts_with("Array<") {
        return (KIND_ARRAY, format!("[{}]", inner_of(cname, "Array<")));
    }
    if cname.starts_with("Map<") {
        return (KIND_MAP, format!("map[{}]", inner_of(cname, "Map<")));
    }
    if cname.starts_with("Channel<") {
        return (KIND_CHAN, format!("chan {}", inner_of(cname, "Channel<")));
    }
    if cname.starts_with("Struct<") {
        return (KIND_STRUCT, inner_of(cname, "Struct<"));
    }
    if cname == "Interface" {
        return (KIND_INTERFACE, "interface{}".to_string());
    }
    if cname == "FunctionObject" {
        return (KIND_FUNC, "func".to_string());
    }
    (KIND_STRUCT, cname.to_string())
}

fn inner_of(s: &str, prefix: &str) -> String {
    if let Some(rest) = s.strip_prefix(prefix)
        && let Some(trimmed) = rest.strip_suffix('>')
    {
        return trimmed.to_string();
    }
    s.to_string()
}

impl StringObject {
    fn from_static(s: &'static str) -> Self {
        StringObject::new(s.as_ptr(), s.len())
    }
}

#[repr(C)]
struct StackFrameReflectTypeKind<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut usize,
    type_id: usize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_reflect_type_kind(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameReflectTypeKind>();

    let kind = if frame.type_id == 0 {
        KIND_INVALID
    } else {
        let tid = TypeId::from_raw(frame.type_id);
        let cname = tid.name().to_str().unwrap_or("").to_string();
        mapping_for_name(&cname).0
    };

    let frame = ctx.stack_frame_mut::<StackFrameReflectTypeKind>();
    *frame.result_ptr = kind as usize;

    ctx.pop_frame()
}

#[repr(C)]
struct StackFrameReflectTypeString<'a> {
    common: StackFrameCommon,
    result_ptr: &'a mut StringObject,
    type_id: usize,
}

#[unsafe(no_mangle)]
pub extern "C" fn gox5_reflect_type_string(ctx: &mut LightWeightThreadContext) -> FunctionObject {
    let frame = ctx.stack_frame::<StackFrameReflectTypeString>();

    let name = if frame.type_id == 0 {
        StringObject::new(ptr::null(), 0)
    } else {
        let tid = TypeId::from_raw(frame.type_id);
        let cname = tid.name().to_str().unwrap_or("").to_string();
        if let Some(m) = mapping_for_basic(&cname) {
            StringObject::from_static(m.go_name)
        } else {
            // Composite/named types need dynamic storage (the C name differs
            // from the Go name), so build a heap-backed string.
            let go_name = mapping_for_name(&cname).1;
            let mut builder = ctx.global_context().process(|mut global_context| {
                StringObject::builder(go_name.len(), global_context.allocator())
            });
            builder.append_bytes(go_name.as_bytes());
            builder.build()
        }
    };

    let frame = ctx.stack_frame_mut::<StackFrameReflectTypeString>();
    *frame.result_ptr = name;

    ctx.pop_frame()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ObjectAllocator;
    use crate::global_context;
    use crate::light_weight_thread::LightWeightThreadContext;
    use std::mem;

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
            let size = size.div_ceil(alignment) * alignment;
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
            self.allocate(num_pages * 4096, |_| {})
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for obj in &self.allocated_objects {
                (obj.destructor)(obj.ptr);
                unsafe {
                    Vec::from_raw_parts(obj.ptr as *mut isize, 0, obj.size);
                }
            }
        }
    }

    fn create_ctx() -> (LightWeightThreadContext, global_context::GlobalContextPtr) {
        let allocator = Box::new(MockObjectAllocator::new());
        let gc = global_context::create_global_context(allocator);
        let func = FunctionObject::new_null();
        let ctx = crate::create_light_weight_thread_context(gc.dupulicate(), func);
        (ctx, gc)
    }

    struct TypeIdGuard {
        blob: *mut [u64],
        tid: usize,
    }

    impl TypeIdGuard {
        fn tid(&self) -> usize {
            self.tid
        }
    }

    impl Drop for TypeIdGuard {
        fn drop(&mut self) {
            unsafe {
                drop(Box::from_raw(self.blob));
            }
        }
    }

    fn type_id_for(cname: &'static [u8]) -> TypeIdGuard {
        let mut raw: Box<[u64]> =
            vec![0u64; crate::type_id::TYPE_INFO_SIZE.div_ceil(8)].into_boxed_slice();
        let s = StringObject::new(cname.as_ptr(), cname.len());
        unsafe {
            (raw.as_mut_ptr() as *mut StringObject).write(s);
        }
        // Box::leak exposes the allocation's provenance, which miri requires so
        // the runtime can re-derive a &TypeInfo from the raw type_id integer.
        // Keep only a raw pointer (not the &'static mut) for later reclaim, to
        // avoid a conflicting live reference during the read.
        let leaked: &'static mut [u64] = Box::leak(raw);
        let blob = leaked as *mut [u64];
        let tid = leaked.as_ptr() as usize;
        TypeIdGuard { blob, tid }
    }

    #[test]
    fn test_reflect_type_kind_basic() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<usize>());
        let result_raw = ctx.stack_pointer() as *mut usize;
        ctx.grow_stack(mem::size_of::<StackFrameReflectTypeKind>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let tid = type_id_for(b"IntObject");
        let frame = ctx.stack_frame_mut::<StackFrameReflectTypeKind>();
        frame.type_id = tid.tid();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_reflect_type_kind(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { *result_raw }, KIND_INT as usize);
    }

    #[test]
    fn test_reflect_type_kind_string() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<usize>());
        let result_raw = ctx.stack_pointer() as *mut usize;
        ctx.grow_stack(mem::size_of::<StackFrameReflectTypeKind>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let tid = type_id_for(b"StringObject");
        let frame = ctx.stack_frame_mut::<StackFrameReflectTypeKind>();
        frame.type_id = tid.tid();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_reflect_type_kind(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { *result_raw }, KIND_STRING as usize);
    }

    #[test]
    fn test_reflect_type_string_basic() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<StringObject>());
        let result_raw = ctx.stack_pointer() as *mut StringObject;
        ctx.grow_stack(mem::size_of::<StackFrameReflectTypeString>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let tid = type_id_for(b"IntObject");
        let frame = ctx.stack_frame_mut::<StackFrameReflectTypeString>();
        frame.type_id = tid.tid();
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_reflect_type_string(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        let name = unsafe { &*result_raw };
        assert_eq!(name.to_str().unwrap(), "int");
    }

    #[test]
    fn test_reflect_type_invalid_zero() {
        let (mut ctx, _gc) = create_ctx();
        let prev_sp = ctx.stack_pointer();
        ctx.grow_stack(mem::size_of::<usize>());
        let result_raw = ctx.stack_pointer() as *mut usize;
        ctx.grow_stack(mem::size_of::<StackFrameReflectTypeKind>());
        ctx.push_frame(
            prev_sp,
            Some(result_raw as *const ()),
            &[],
            FunctionObject::new_null(),
        );

        let frame = ctx.stack_frame_mut::<StackFrameReflectTypeKind>();
        frame.type_id = 0;
        frame.result_ptr = unsafe { &mut *result_raw };

        let result = gox5_reflect_type_kind(&mut ctx);
        assert_eq!(result, FunctionObject::new_null());
        assert_eq!(unsafe { *result_raw }, KIND_INVALID as usize);
    }
}
