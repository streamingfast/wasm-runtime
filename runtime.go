package wasm

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"reflect"
	"unicode/utf16"

	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
)

type abortError struct {
	message      string
	filename     string
	lineNumber   int
	columnNumber int
}

func (e *abortError) Error() string {
	return fmt.Sprintf("wasm execution aborted at %s:%d:%d: %s", e.filename, e.lineNumber, e.columnNumber, e.message)
}

type Environment interface {
	SetMemory(memory *wasmer.Memory)

	ReadBytes(offset int32) ([]byte, error)
	ReadString(offset int32) (string, error)

	// We should probably have a ReadSlice receiving some reflect. Type to deal with all the possibilities
	ReadI32s(offset int32) ([]int32, error)
	ReadStrings(offset int32) ([]string, error)

	LogSegment(message string, offset int32, length int32)
	RecordCall(module, function string, params []interface{}, returns interface{})
}

type CallRecorder interface {
	Record(module, name string, params []interface{}, returnValue interface{})
}

var emptyEnvironment = &DefaultEnvironment{}

type DefaultEnvironment struct {
	CallRecorder CallRecorder
	memory       *wasmer.Memory
}

var encoding = binary.LittleEndian
var bigEncoding = binary.BigEndian

func (e *DefaultEnvironment) SetMemory(memory *wasmer.Memory) {
	e.memory = memory
}

func (e *DefaultEnvironment) dataAt(offset int32) ([]byte, error) {
	bytes := e.memory.Data()
	if offset < 0 {
		return nil, fmt.Errorf("offset %d must be positive", offset)
	}

	if offset > int32(len(bytes)) {
		return nil, fmt.Errorf("offset %d out of memory bounds ending at %d", offset, len(bytes))
	}

	return e.memory.Data()[offset:], nil
}

func (e *DefaultEnvironment) segment(offset int32, length int32) ([]byte, error) {
	bytes := e.memory.Data()
	if offset < 0 {
		return nil, fmt.Errorf("offset %d must be positive", offset)
	}

	if offset >= int32(len(bytes)) {
		return nil, fmt.Errorf("offset %d out of memory bounds ending at %d", offset, len(bytes))
	}

	end := offset + length
	if end > int32(len(bytes)) {
		return nil, fmt.Errorf("end %d out of memory bounds ending at %d", end, len(bytes))
	}

	return bytes[offset : offset+length], nil
}

func (e *DefaultEnvironment) ReadString(offset int32) (string, error) {
	e.LogSegment("Data +size type?", offset-12, 16)

	characterCount, err := e.readI32("string length", offset)
	if err != nil {
		return "", fmt.Errorf("read length: %w", err)
	}

	offset += 4
	bytes, err := e.segment(offset, characterCount*2)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	if ztracer.Enabled() {
		zlog.Debug("read string content bytes", zap.Stringer("bytes", hexBytes(bytes)))
	}

	characters := make([]uint16, characterCount)
	for i := int32(0); i < characterCount; i++ {
		offset := i * 2
		characters[i] = uint16(bytes[offset+1])<<8 | uint16(bytes[offset])
	}

	return string(utf16.Decode(characters)), nil
}

func (e *DefaultEnvironment) LogSegment(message string, offset int32, length int32) {
	// a82700000000000003000000000000000300000000000000
	// e6f5af

	// a8270000 00000000 03000000   000000000300000000000000
	// e6f5af

	bytes, err := e.segment(offset, length)
	if err != nil {
		zlog.Info("unable to obtain data segment %d to %d for "+message, zap.Error(err))
	} else {
		zlog.Info(message, zap.Stringer("bytes", hexBytes(bytes)))
	}
}

// ReadBytes implements the logic for https://www.assemblyscript.org/memory.html#array-layout
// which from my understanding of the spec, is looking like this:
//
// 4 bytes - Offset where array data is located, should be jump to
// 4 bytes - Data start offset within the array data above, should skip this value before reading elements
// 4 bytes - Data size in bytes to read
// 12 bytes - Mutable length of the data the user is interested in, typed as i32, I don't understand how this fields work actually, ignored for now
func (e *DefaultEnvironment) ReadBytes(offset int32) (out []byte, err error) {
	arrayOffset, err := e.readU32("array offset", offset)
	if err != nil {
		return nil, err
	}

	dataStart, err := e.readU32("data start", offset+4)
	if err != nil {
		return nil, err
	}

	dataSize, err := e.readU32("data size", offset+8)
	if err != nil {
		return nil, err
	}

	e.LogSegment("array offset 1", offset, 32)
	e.LogSegment("array offset 2", 10152, 32)

	return e.segment(int32(arrayOffset+dataStart), int32(dataSize))
}

func (e *DefaultEnvironment) ReadI32s(offset int32) (out []int32, err error) {
	arrayOffset, err := e.readI32("i32 array offset", offset)
	if err != nil {
		return nil, fmt.Errorf("read i32 array offset: %w", err)
	}

	length, err := e.readI32("i32 array length", offset+4)
	if err != nil {
		return nil, fmt.Errorf("read i32 array length: %w", err)
	}

	if ztracer.Enabled() {
		zlog.Debug("resolving i32 array reference", zap.Int32("offset", arrayOffset), zap.Int32("length", length))
	}

	// Gives 0800000000000000 (0000000000000008 in big endian), not sure of the meaning actually
	_, err = e.readI64("i32 array field", arrayOffset)

	indicesOffset := arrayOffset + 8
	sizeOfI32 := int32(4)
	out = make([]int32, length)
	for i := int32(0); i < length; i++ {
		out[i], err = e.readI32("i32 array element", indicesOffset+(i*sizeOfI32))
		if err != nil {
			return nil, fmt.Errorf("read i32 array index #%d: %w", i, err)
		}
	}

	return out, nil
}

func (e *DefaultEnvironment) ReadStrings(offset int32) (out []string, err error) {
	arrayOffset, err := e.readI32("string array offset", offset)
	if err != nil {
		return nil, fmt.Errorf("read string array offset: %w", err)
	}

	length, err := e.readI32("string array length", offset+4)
	if err != nil {
		return nil, fmt.Errorf("read string array length: %w", err)
	}

	if ztracer.Enabled() {
		zlog.Debug("resolving string array reference", zap.Int32("offset", arrayOffset), zap.Int32("length", length))
	}

	// Gives 0800000000000000 (0000000000000008 in big endian), not sure of the meaning actually
	_, err = e.readI64("string array field", arrayOffset)

	indicesOffset := arrayOffset + 8
	sizeOfString := int32(4)
	out = make([]string, length)
	for i := int32(0); i < length; i++ {
		stringOffset, err := e.readI32("string array element offset", indicesOffset+(i*sizeOfString))
		if err != nil {
			return nil, fmt.Errorf("read string array index #%d offset: %w", i, err)
		}

		out[i], err = e.ReadString(stringOffset)
		if err != nil {
			return nil, fmt.Errorf("read string array index #%d: %w", i, err)
		}
	}

	return out, nil
}

func (e *DefaultEnvironment) readI32(tag string, offset int32) (int32, error) {
	bytes, err := e.segment(offset, 4)
	if err != nil {
		return 0, err
	}

	// It's an actual i32 here, should we parse it differently than a Uint32?
	out := int32(encoding.Uint32(bytes))

	if ztracer.Enabled() {
		zlog.Debug("read "+tag+" i32 bytes", zap.Int32("value", out), zap.Stringer("bytes", hexBytes(bytes)), zap.Int32("offset", offset))
	}

	return out, nil
}

func (e *DefaultEnvironment) readU32(tag string, offset int32) (uint32, error) {
	bytes, err := e.segment(offset, 4)
	if err != nil {
		return 0, err
	}

	out := encoding.Uint32(bytes)
	if ztracer.Enabled() {
		zlog.Debug("read "+tag+" u32 bytes", zap.Uint32("value", out), zap.Stringer("bytes", hexBytes(bytes)), zap.Int32("offset", offset))
	}

	return out, nil
}

func (e *DefaultEnvironment) readI64(tag string, offset int32) (int64, error) {
	bytes, err := e.segment(offset, 8)
	if err != nil {
		return 0, err
	}

	// It's an actual i64 here, should we parse it differently than a Uint64?
	out := int64(encoding.Uint64(bytes))

	if ztracer.Enabled() {
		zlog.Debug("read "+tag+" i64 bytes", zap.Int64("value", out), zap.Stringer("bytes", hexBytes(bytes)))
	}

	return out, nil
}

func (e *DefaultEnvironment) RecordCall(module, function string, params []interface{}, returns interface{}) {
	if e.CallRecorder != nil {
		e.CallRecorder.Record(module, function, params, returns)
	}
}

func (e *DefaultEnvironment) Debug() string {
	if e.memory == nil {
		return "<empty>"
	}

	return hex.EncodeToString(e.memory.Data())
}

type MemoryAllocationFactory func(instance *wasmer.Instance) wasmer.NativeFunction
type RuntimeOption func(*Runtime)

func WithMemoryAllocationFactory(factory MemoryAllocationFactory) RuntimeOption {
	return func(r *Runtime) {
		r.memoryAllocFactory = factory
	}
}

func WithParameterPointSize() RuntimeOption {
	return func(r *Runtime) {
		r.pointerWithSize = true
	}
}

type Runtime struct {
	env                Environment
	memoryAllocFactory MemoryAllocationFactory
	pointerWithSize    bool
}

func NewRuntime(env Environment, options ...RuntimeOption) *Runtime {
	runtime := &Runtime{
		env: env,
	}

	for _, option := range options {
		option(runtime)
	}
	return runtime
}

//Deprecated
func (r *Runtime) Simulate(wasmFile string, entrypointName string, returns reflect.Type, parameters ...interface{}) (interface{}, error) {
	return r.Execute(wasmFile, entrypointName, returns, parameters)
}

func (r *Runtime) Execute(wasmFile string, functionName string, returns reflect.Type, parameters ...interface{}) (interface{}, error) {
	wasmBytes, err := ioutil.ReadFile(wasmFile)
	if err != nil {
		return nil, fmt.Errorf("unable to load wasm file %q: %w", wasmFile, err)
	}

	engine := wasmer.NewEngine()
	store := wasmer.NewStore(engine)

	module, err := wasmer.NewModule(store, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("unable to compile wasm file %q: %w", wasmFile, err)
	}

	importObject := newImports(r.env, store)
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		return nil, fmt.Errorf("unable to get wasm module instance from %q: %w", wasmFile, err)
	}

	memory, err := instance.Exports.GetMemory("memory")
	if err != nil {
		return nil, fmt.Errorf("unable to get the wasm module memory: %w", err)
	}

	r.env.SetMemory(memory)

	if ztracer.Enabled() {
		pages := memory.Size()

		zlog.Debug("memory information for invocation",
			zap.Uint32("pages_count", pages.ToUint32()),
			zap.Uint("pages_bytes", pages.ToBytes()),
			zap.Uint("date_size_bytes", memory.DataSize()),
		)
	}

	entrypointFunction, err := instance.Exports.GetRawFunction(functionName)
	if err != nil {
		return nil, fmt.Errorf("unable to get wasm module function %q from %q: %w", functionName, wasmFile, err)
	}

	if ztracer.Enabled() {
		zlog.Debug("entrypoint function loaded", zap.Stringer("def", namedFunctionDefinition{functionName, entrypointFunction}))
	}

	heap := &AscHeap{
		memory:        memory,
		arenaFreeSize: len(memory.Data()),
	}
	if r.memoryAllocFactory != nil {
		heap.allocator = r.memoryAllocFactory(instance)
	}

	result, err := r.callFunction(heap, entrypointFunction, parameters...)

	if err != nil {
		return nil, fmt.Errorf("unable to execute wasm module function %q from %q: %w", functionName, wasmFile, err)
	}

	//if traceMemoryEnabled {
	//	fmt.Println(env.(*DefaultEnvironment).Debug())
	//}

	//getAt, err := instance.Exports.GetFunction("get_at")
	//
	//if err != nil {
	//	return nil, fmt.Errorf("no get at")
	//}
	//
	//fmt.Println(getAt(result))

	zlog.Info("execution result", zap.Reflect("result", result))
	return toGoValue(result, returns, r.env)
}

func toGoValue(wasmValue interface{}, returns reflect.Type, env Environment) (interface{}, error) {
	if returns == nil {
		return wasmValue, nil
	}

	switch returns.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Already converted by Wasmer
		return wasmValue, nil

	case reflect.Slice:
		if returns.Elem().Kind() == reflect.Uint8 {
			out, err := env.ReadBytes(wasmValue.(int32))
			if err != nil {
				return nil, fmt.Errorf("read bytes: %w", err)
			}

			return out, nil
		}

		// FIXME: Deals with all kind of arrays?
		return nil, fmt.Errorf("unhandled return kind slice of %s", returns.Elem().Kind())

	case reflect.String:
		out, err := env.ReadString(wasmValue.(int32))
		if err != nil {
			return nil, fmt.Errorf("read string: %w", err)
		}

		return out, nil
	default:
		return nil, fmt.Errorf("unhandled return kind %s", returns.Kind())
	}
}

type AscHeap struct {
	memory        *wasmer.Memory
	allocator     wasmer.NativeFunction
	arenaStartPtr int32
	arenaFreeSize int
}

const MIN_ARENA_SIZE = 10000

func (h *AscHeap) Write(bytes []byte) int32 {
	size := len(bytes)
	if size > h.arenaFreeSize {
		newSize := size
		if newSize < MIN_ARENA_SIZE {
			newSize = MIN_ARENA_SIZE
		}

		result, err := h.allocator(int32(newSize))
		if err != nil {
			panic(fmt.Errorf("unable to allocate memory: %w", err)) //todo: why? pas de panic tabar...
		}

		h.arenaStartPtr = result.(int32)
		h.arenaFreeSize = newSize

		zlog.Debug("memory allocated", zap.Int32("arena_start_ptr", h.arenaStartPtr), zap.Int("arena_free_size", newSize))
	}

	ptr := h.arenaStartPtr
	memoryData := h.memory.Data()
	copy(memoryData[ptr:], bytes)

	h.arenaStartPtr += int32(size)
	h.arenaFreeSize -= size

	if ztracer.Enabled() {
		zlog.Debug("memory object written", zap.Int32("data_ptr", ptr), zap.Int32("arena_start_ptr", h.arenaStartPtr), zap.Int("arena_free_size", h.arenaFreeSize))
	}

	return ptr
}

type AscPtr interface {
	ToPtr(heap *AscHeap) (int32, int32)
}

type AscString string

func (h AscString) ToPtr(heap *AscHeap) (int32, int32) {
	// 4 bytes for the lenght of the string, len * 2 (each character is encoded as a uint16)
	size := 4 + len(h)*2
	bytes := make([]byte, size)

	encoding.PutUint32(bytes, uint32(len(h)))
	stringBytes := bytes[4:]

	characters := utf16.Encode([]rune(h))
	for i, b := range characters {
		offset := i * 2

		stringBytes[offset] = byte(b & 0x00FF)
		stringBytes[offset+1] = byte((b & 0xFF00) >> 8)
	}

	return heap.Write(bytes), int32(size)
}

type AscBytes []byte

func (h AscBytes) ToPtr(heap *AscHeap) (int32, int32) {
	// 4 bytes for the length of the bytes + len (each character is encoded as a uint8)
	size := 4 + len(h)
	bytes := make([]byte, size)

	encoding.PutUint32(bytes, uint32(len(h)))
	dataBytes := bytes[4:]
	for i, b := range h {
		dataBytes[i] = byte(b)
	}

	ptr := heap.Write(bytes)

	return ptr, int32(size)
}

func (r *Runtime) callFunction(heap *AscHeap, entrypoint *wasmer.Function, parameters ...interface{}) (out interface{}, err error) {
	//defer func() {
	//	if r := recover(); r != nil {
	//		switch x := r.(type) {
	//		case string:
	//			err = errors.New(x)
	//		case error:
	//			err = x
	//		default:
	//			err = errors.New("Unknown panic")
	//		}
	//	}
	//}()
	wasmParameters := toWASMParameters(heap, parameters, r.pointerWithSize)

	return entrypoint.Call(wasmParameters...)
}

func toWASMParameters(heap *AscHeap, parameters []interface{}, withSize bool) (out []interface{}) {
	for _, parameter := range parameters {
		wasmValue := toWASMValue(parameter)
		size := int32(math.MaxInt32) //not super clean
		if v, ok := wasmValue.(AscPtr); ok {
			wasmValue, size = v.ToPtr(heap)
		}

		if ztracer.Enabled() {
			zlog.Debug("converted parameter to wasm value", zap.Stringer("original", typedField{parameter}), zap.Stringer("wasm", typedField{wasmValue}))
		}

		out = append(out, wasmValue)

		if withSize {
			if size != int32(math.MaxInt32) {
				out = append(out, size)
			}
		}
	}

	return
}

func toWASMValue(in interface{}) interface{} {
	switch v := in.(type) {
	case bool:
		if v == true {
			return int32(1)
		}

		return int32(0)
	case int8:
		return int32(v)
	case uint8:
		return int32(v)
	case int16:
		return int32(v)
	case uint16:
		return int32(v)
	case int32:
		return int32(v)
	case uint32:
		return int32(v)
	case int64:
		return int64(v)
	case uint64:
		return uint64(v)
	case int:
		// The WASM spec differentiates between int32 vs int64 depending on WASM32 or WASM64, but I assume we are always in the context of WASM32 here
		return int32(v)
	case uint:
		// The WASM spec differentiates between int32 vs int64 depending on WASM32 or WASM64, but I assume we are always in the context of WASM32 here
		return int32(v)
	case float32, float64:
		return v

	case []byte:
		return AscBytes(v)
	case string:
		return AscString(v)
	}

	panic(fmt.Errorf("unhandled type %T to WASM", in))
}

type hexBytes []byte

func (h hexBytes) String() string {
	return hex.EncodeToString(h)
}

type typedField struct {
	value interface{}
}

func (f typedField) String() string {
	return reflect.TypeOf(f.value).String()
}
