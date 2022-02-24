package wasm

import (
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
		panic("must handle length here") // FIXME
		//out, err := env.ReadString(wasmValue.(int32), 0)
		//if err != nil {
		//	return nil, fmt.Errorf("read string: %w", err)
		//}
		//return out, nil

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
