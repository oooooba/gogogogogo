#define _GNU_SOURCE
#include <assert.h>
#include <complex.h>
#include <fcntl.h>
#include <math.h>
#include <stdatomic.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define DECLARE_RUNTIME_API(name, param_type)                                  \
    FunctionObject(gox5_##name)(LightWeightThreadContext * ctx)

typedef struct GlobalContext GlobalContext;

typedef struct {
    const void *func_ptr;
} UserFunction;

struct TypeInfo;

typedef union {
    uintptr_t id;
    const struct TypeInfo *info;
} TypeId;

#define DEFINE_BUILTIN_OBJECT_TYPE(name, raw_type)                             \
    typedef struct {                                                           \
        raw_type raw;                                                          \
    } name##Object

DEFINE_BUILTIN_OBJECT_TYPE(Bool, bool);
DEFINE_BUILTIN_OBJECT_TYPE(Complex64, float complex);
DEFINE_BUILTIN_OBJECT_TYPE(Complex128, double complex);
DEFINE_BUILTIN_OBJECT_TYPE(Float32, float);
DEFINE_BUILTIN_OBJECT_TYPE(Float64, double);
DEFINE_BUILTIN_OBJECT_TYPE(Int, intptr_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int8, int8_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int16, int16_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int32, int32_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int64, int64_t);
DEFINE_BUILTIN_OBJECT_TYPE(UnsafePointer, void *);
DEFINE_BUILTIN_OBJECT_TYPE(Uint, uintptr_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint8, uint8_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint16, uint16_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint32, uint32_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint64, uint64_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uintptr, uintptr_t);

typedef struct {
    void *raw;
} ChannelObject;

typedef struct {
    const void *raw;
} FunctionObject;

typedef struct {
    const char *raw;
    uintptr_t len;
} StringObject;

typedef struct {
    void *raw;
} MapObject;

typedef struct {
} InvalidObject;

typedef struct {
    union {
        MapObject map;
        StringObject string;
    } obj;
    uintptr_t count;
} IterObject;

typedef struct StackFrameCommon {
    FunctionObject resume_func;
    struct StackFrameCommon *prev_stack_pointer;
    void *free_vars;
    const void *deferred_list;
} StackFrameCommon;

typedef struct {
    StackFrameCommon *stack_pointer;
    UserFunction prev_func;
    intptr_t marker;
} LightWeightThreadContext;

typedef struct {
    StringObject method_name;
    FunctionObject method;
    StringObject method_signature;
} InterfaceTableEntry;

typedef struct TypeInfo {
    StringObject name;
    uintptr_t num_methods;
    const InterfaceTableEntry *interface_table;
    void *is_equal;
    void *hash;
    uintptr_t size;
} TypeInfo;

typedef struct {
    void *receiver;
    TypeId type_id;
} Interface;

typedef struct {
    void *addr;
    uintptr_t size;
    uintptr_t capacity;
} SliceObject;

typedef struct {
    uintptr_t pc;
    StringObject name;
} UserFunctionInfo;

const UserFunctionInfo *gox5_runtime_func_for_pc(uintptr_t pc);
StringObject gox5_runtime_func_name(const UserFunctionInfo *func);

typedef struct {
    StackFrameCommon common;
    SliceObject *result_ptr;
    TypeId type_id;
    SliceObject lhs;
    SliceObject rhs;
} StackFrameSliceAppend;
DECLARE_RUNTIME_API(slice_append, StackFrameSliceAppend);

typedef struct {
    StackFrameCommon common;
    SliceObject *result_ptr;
    SliceObject slice;
    StringObject string;
} StackFrameSliceAppendString;
DECLARE_RUNTIME_API(slice_append_string, StackFrameSliceAppendString);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject slice;
} StackFrameSliceCapacity;
DECLARE_RUNTIME_API(slice_capacity, StackFrameSliceCapacity);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject lhs;
    SliceObject rhs;
} StackFrameSliceCompare;
DECLARE_RUNTIME_API(slice_compare, StackFrameSliceCompare);

typedef struct {
    StackFrameCommon common;
    uintptr_t *result_ptr;
    uintptr_t type_id;
} StackFrameReflectTypeKind;
DECLARE_RUNTIME_API(reflect_type_kind, StackFrameReflectTypeKind);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    uintptr_t type_id;
} StackFrameReflectTypeString;
DECLARE_RUNTIME_API(reflect_type_string, StackFrameReflectTypeString);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    TypeId type_id;
    SliceObject src;
    SliceObject dst;
} StackFrameSliceCopy;
DECLARE_RUNTIME_API(slice_copy, StackFrameSliceCopy);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    StringObject src;
    SliceObject dst;
} StackFrameSliceCopyString;
DECLARE_RUNTIME_API(slice_copy_string, StackFrameSliceCopy);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject b;
    Uint8Object c;
} StackFrameSliceCount;
DECLARE_RUNTIME_API(slice_count, StackFrameSliceCount);

typedef struct {
    StackFrameCommon common;
    SliceObject *result_ptr;
    TypeId type_id;
    StringObject src;
} StackFrameSliceFromString;
DECLARE_RUNTIME_API(slice_from_string, StackFrameSliceFromString);

typedef struct {
    StackFrameCommon common;
    SliceObject *result_ptr;
    IntObject n;
} StackFrameSliceNewUninitialized;
DECLARE_RUNTIME_API(slice_new_uninitialized, StackFrameSliceNewUninitialized);
typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject b;
    Uint8Object c;
} StackFrameSliceSearchByte;
DECLARE_RUNTIME_API(slice_search_byte, StackFrameSliceSearchByte);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject lhs;
    SliceObject rhs;
} StackFrameSliceSearchSlice;
DECLARE_RUNTIME_API(slice_search_slice, StackFrameSliceSearchSlice);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    SliceObject slice;
} StackFrameSliceSize;
DECLARE_RUNTIME_API(slice_size, StackFrameSliceSize);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    StringObject lhs;
    StringObject rhs;
} StackFrameStringAppend;
DECLARE_RUNTIME_API(string_append, StackFrameStringAppend);

typedef struct {
    StackFrameCommon common;
    StringObject string;
    IntObject *index;
    Int32Object *rune;
    bool *found;
    uintptr_t *count;
} StackFrameStringNext;
DECLARE_RUNTIME_API(string_next, StackFrameStringNext);

typedef struct {
    StackFrameCommon common;
    FunctionObject function_object;
    uintptr_t result_size;
    uintptr_t num_arg_buffer_words;
    void *arg_buffer[0];
} StackFrameDeferRegister;
DECLARE_RUNTIME_API(defer_register, StackFrameDeferRegister);

typedef struct {
    StackFrameCommon common;
    Interface *interface;
    StringObject method_name;
    uintptr_t result_size;
    uintptr_t num_arg_buffer_words;
    void *arg_buffer[0];
} StackFrameDeferRegisterInvoke;
DECLARE_RUNTIME_API(defer_register_invoke, StackFrameDeferRegisterInvoke);

typedef struct {
    StackFrameCommon common;
    ChannelObject *result_ptr;
    TypeId type_id;
    IntObject capacity; // ToDo: correct to proper type
} StackFrameChannelNew;
DECLARE_RUNTIME_API(channel_new, StackFrameChannelNew);

typedef struct {
    StackFrameCommon common;
    IntObject *selected_index;
    BoolObject *receive_available;
    uintptr_t need_block;
    uintptr_t entry_count;
    struct {
        ChannelObject channel;
        TypeId type_id;
        const void *send_data;
        void *receive_data;
    } entry_buffer[0];
} StackFrameChannelSelect;
DECLARE_RUNTIME_API(channel_select, StackFrameChannelSelect);

typedef struct {
    StackFrameCommon common;
    void *result_ptr;
    void *pointer;
} StackFrameCheckNonNil;

__attribute__((unused)) static FunctionObject
gox5_check_non_nil(LightWeightThreadContext *ctx) {
    StackFrameCheckNonNil *frame = (void *)ctx->stack_pointer;
    assert(frame->pointer != NULL);
    *((void **)frame->result_ptr) = frame->pointer;
    ctx->stack_pointer = frame->common.prev_stack_pointer;
    return frame->common.resume_func;
}

typedef struct {
    StackFrameCommon common;
    FunctionObject *result_ptr;
    UserFunction user_function;
    uintptr_t num_object_ptrs;
    void *object_ptrs[0];
} StackFrameClosureNew;
DECLARE_RUNTIME_API(closure_new, StackFrameClosureNew);

typedef struct {
    StackFrameCommon common;
    void *result_ptr;
    FunctionObject function_object;
} StackFrameCoroNew;
DECLARE_RUNTIME_API(coro_new, StackFrameCoroNew);

typedef struct {
    StackFrameCommon common;
    void *coro;
} StackFrameCoroSwitch;
DECLARE_RUNTIME_API(coro_switch, StackFrameCoroSwitch);

typedef struct {
    StackFrameCommon common;
    Complex64Object *result_ptr;
    Float32Object real;
    Float32Object imaginary;
} StackFrameComplex64New;

__attribute__((unused)) static FunctionObject
gox5_complex64_new(LightWeightThreadContext *ctx) {
    StackFrameComplex64New *frame = (void *)ctx->stack_pointer;
    float parts[2];
    parts[0] = frame->real.raw;
    parts[1] = frame->imaginary.raw;
    Complex64Object result;
    memcpy(&result, parts, sizeof(result));
    *frame->result_ptr = result;
    ctx->stack_pointer = frame->common.prev_stack_pointer;
    return frame->common.resume_func;
}

typedef struct {
    StackFrameCommon common;
    Float32Object *result_ptr;
    Complex64Object value;
    uintptr_t is_real;
} StackFrameComplex64Component;

__attribute__((unused)) static FunctionObject
gox5_complex64_component(LightWeightThreadContext *ctx) {
    StackFrameComplex64Component *frame = (void *)ctx->stack_pointer;
    *frame->result_ptr = (Float32Object){
        .raw = (frame->is_real ? creal : cimag)(frame->value.raw)};
    ctx->stack_pointer = frame->common.prev_stack_pointer;
    return frame->common.resume_func;
}

typedef struct {
    StackFrameCommon common;
    Complex128Object *result_ptr;
    Float64Object real;
    Float64Object imaginary;
} StackFrameComplex128New;

__attribute__((unused)) static FunctionObject
gox5_complex128_new(LightWeightThreadContext *ctx) {
    StackFrameComplex128New *frame = (void *)ctx->stack_pointer;
    double parts[2];
    parts[0] = frame->real.raw;
    parts[1] = frame->imaginary.raw;
    Complex128Object result;
    memcpy(&result, parts, sizeof(result));
    *frame->result_ptr = result;
    ctx->stack_pointer = frame->common.prev_stack_pointer;
    return frame->common.resume_func;
}

typedef struct {
    StackFrameCommon common;
    Float64Object *result_ptr;
    Complex128Object value;
    uintptr_t is_real;
} StackFrameComplex128Component;

__attribute__((unused)) static FunctionObject
gox5_complex128_component(LightWeightThreadContext *ctx) {
    StackFrameComplex128Component *frame = (void *)ctx->stack_pointer;
    *frame->result_ptr = (Float64Object){
        .raw = (frame->is_real ? creal : cimag)(frame->value.raw)};
    ctx->stack_pointer = frame->common.prev_stack_pointer;
    return frame->common.resume_func;
}

typedef struct {
    StackFrameCommon common;
    Interface *result_ptr;
    const void *receiver;
    TypeId type_id;
} StackFrameInterfaceNew;
DECLARE_RUNTIME_API(interface_new, StackFrameInterfaceNew);

typedef struct {
    StackFrameCommon common;
    void *result_ptr;
    Interface *interface;
    StringObject method_name;
    uintptr_t num_arg_buffer_words;
    void *arg_buffer[0];
} StackFrameInterfaceInvoke;
DECLARE_RUNTIME_API(interface_invoke, StackFrameInterfaceInvoke);

typedef struct {
    StackFrameCommon common;
    Interface *interface;
    TypeId to_type;
    void *value;
    bool *success;
} StackFrameInterfaceConvertToConcreteType;
DECLARE_RUNTIME_API(interface_convert_to_concrete_type,
                    StackFrameInterfaceConvertToConcreteType);

typedef struct {
    StackFrameCommon common;
    Interface *interface;
    TypeId to_type;
    void *value;
    bool *success;
} StackFrameInterfaceConvertToInterface;
DECLARE_RUNTIME_API(interface_convert_to_interface,
                    StackFrameInterfaceConvertToInterface);

typedef struct {
    StackFrameCommon common;
    MapObject *result_ptr;
    TypeId key_type;
    TypeId value_type;
} StackFrameMapNew;
DECLARE_RUNTIME_API(map_new, StackFrameMapNew);

typedef struct {
    StackFrameCommon common;
    Interface *result_ptr;
    Interface map;
} StackFrameMapClone;
DECLARE_RUNTIME_API(map_clone, StackFrameMapClone);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    SliceObject byte_slice;
} StackFrameStringNewFromByteSlice;
DECLARE_RUNTIME_API(string_new_from_byte_slice,
                    StackFrameStringNewFromByteSlice);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    StringObject string;
    Uint8Object byte;
} StackFrameStringSearchByte;
DECLARE_RUNTIME_API(string_search_byte, StackFrameStringSearchByte);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    StringObject lhs;
    StringObject rhs;
} StackFrameStringSearchString;
DECLARE_RUNTIME_API(string_search_string, StackFrameStringSearchString);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    IntObject rune;
} StackFrameStringNewFromRune;
DECLARE_RUNTIME_API(string_new_from_rune, StackFrameStringNewFromRune);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    SliceObject rune_slice;
} StackFrameStringNewFromRuneSlice;
DECLARE_RUNTIME_API(string_new_from_rune_slice,
                    StackFrameStringNewFromRuneSlice);

typedef struct {
    StackFrameCommon common;
    MapObject map;
} StackFrameMapClear;
DECLARE_RUNTIME_API(map_clear, StackFrameMapClear);

typedef struct {
    StackFrameCommon common;
    MapObject map;
    const void *key;
} StackFrameMapDelete;
DECLARE_RUNTIME_API(map_delete, StackFrameMapDelete);

typedef struct {
    StackFrameCommon common;
    MapObject map;
    const void *key;
    void *value;
    bool *found;
} StackFrameMapGet;
DECLARE_RUNTIME_API(map_get, StackFrameMapGet);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    MapObject map;
} StackFrameMapLen;
DECLARE_RUNTIME_API(map_len, StackFrameMapLen);

typedef struct {
    StackFrameCommon common;
    MapObject map;
    const void *key;
    void *value;
    bool *found;
    uintptr_t *count;
} StackFrameMapNext;
DECLARE_RUNTIME_API(map_next, StackFrameMapNext);

typedef struct {
    StackFrameCommon common;
    MapObject map;
    const void *key;
    const void *value;
} StackFrameMapSet;
DECLARE_RUNTIME_API(map_set, StackFrameMapSet);

typedef struct {
    StackFrameCommon common;
    void *result_ptr;
    uintptr_t size;
} StackFrameNew;
DECLARE_RUNTIME_API(new, StackFrameNew);

typedef struct {
    StackFrameCommon common;
    ChannelObject channel;
} StackFrameChannelClose;
DECLARE_RUNTIME_API(channel_close, StackFrameChannelClose);

typedef struct {
    StackFrameCommon common;
    ChannelObject channel;
    TypeId type_id;
    void *data;
    bool *available;
} StackFrameChannelReceive;
DECLARE_RUNTIME_API(channel_receive, StackFrameChannelReceive);

typedef struct {
    StackFrameCommon common;
} StackFrameDeferExecute;
DECLARE_RUNTIME_API(defer_execute, StackFrameDeferExecute);

typedef struct {
    StackFrameCommon common;
    ChannelObject channel;
    const void *data;
    TypeId type_id;
} StackFrameChannelSend;
DECLARE_RUNTIME_API(channel_send, StackFrameChannelSend);
typedef struct {
    StackFrameCommon common;
    Uint32Object *s;
} StackFrameSemaphoreAcquire;
DECLARE_RUNTIME_API(semaphore_acquire, StackFrameSemaphoreAcquire);

typedef struct {
    StackFrameCommon common;
    Uint32Object *s;
} StackFrameSemaphoreRelease;
DECLARE_RUNTIME_API(semaphore_release, StackFrameSemaphoreRelease);

typedef struct {
    StackFrameCommon common;
    Interface value;
} StackFramePanicRaise;
DECLARE_RUNTIME_API(panic_raise, StackFramePanicRaise);

typedef struct {
    StackFrameCommon common;
    Interface *result_ptr;
} StackFramePanicRecover;
DECLARE_RUNTIME_API(panic_recover, StackFramePanicRecover);

typedef struct {
    StackFrameCommon common;
} StackFrameLwtExit;
DECLARE_RUNTIME_API(lwt_exit, StackFrameLwtExit);

typedef struct {
    StackFrameCommon common;
    FunctionObject function_object;
    uintptr_t result_size;
    uintptr_t num_arg_buffer_words;
    void *arg_buffer[0];
} StackFrameLwtSpawn;
DECLARE_RUNTIME_API(lwt_spawn, StackFrameLwtSpawn);

typedef struct {
    StackFrameCommon common;
    Interface *interface;
    StringObject method_name;
    uintptr_t result_size;
    uintptr_t num_arg_buffer_words;
    void *arg_buffer[0];
} StackFrameLwtSpawnInvoke;
DECLARE_RUNTIME_API(lwt_spawn_invoke, StackFrameLwtSpawnInvoke);

typedef struct {
    StackFrameCommon common;
} StackFrameLwtYield;
DECLARE_RUNTIME_API(lwt_yield, StackFrameLwtYield);

typedef struct {
    StackFrameCommon common;
    Uint64Object *result_ptr;
} StackFrameRuntimeRand;
DECLARE_RUNTIME_API(runtime_rand, StackFrameRuntimeRand);

typedef struct {
    StackFrameCommon common;
    IntObject *result_ptr;
    StringObject string;
} StackFrameStringLength;
DECLARE_RUNTIME_API(string_length, StackFrameStringLength);

typedef struct {
    StackFrameCommon common;
    StringObject *result_ptr;
    StringObject base;
    intptr_t low;
    intptr_t high;
} StackFrameStringSubstr;
DECLARE_RUNTIME_API(string_substr, StackFrameStringSubstr);

static inline int string_compare(const StringObject *lhs,
                                 const StringObject *rhs) {
    uintptr_t min_len = lhs->len < rhs->len ? lhs->len : rhs->len;
    int result = 0;
    if (min_len != 0 && lhs->raw != NULL && rhs->raw != NULL) {
        result = memcmp(lhs->raw, rhs->raw, min_len);
    }
    if (result != 0) {
        return result;
    }
    if (lhs->len < rhs->len) {
        return -1;
    }
    if (lhs->len > rhs->len) {
        return 1;
    }
    return 0;
}
