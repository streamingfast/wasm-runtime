package wasm

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"reflect"

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
	return fmt.Sprintf("wasm execution aborted at %s:%d env:%d env: %s", e.filename, e.lineNumber, e.columnNumber, e.message)
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

func (r *Runtime) Execute(wasmFile string, functionName string, parameters []interface{}, returns ...*AscReturnValue) (interface{}, error) {
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

	heap := newAscHeap(memory)
	if r.memoryAllocFactory != nil {
		heap.allocator = r.memoryAllocFactory(instance)
	}

	result, err := r.callFunction(heap, entrypointFunction, parameters, returns)
	if err != nil {
		return nil, fmt.Errorf("unable to execute wasm module function %q from %q: %w", functionName, wasmFile, err)
	}

	zlog.Info("execution result", zap.Reflect("result", result))
	return result, nil
}

type AscHeap struct {
	memory          *wasmer.Memory
	allocator       wasmer.NativeFunction
	nextPtrLocation int32
	freeSpace       uint
}

func newAscHeap(memory *wasmer.Memory) *AscHeap {
	if len(memory.Data()) != int(memory.DataSize()) {
		panic("ALSKDJ")
	}
	return &AscHeap{
		memory:    memory,
		freeSpace: memory.DataSize(),
	}
}

func (h *AscHeap) Write(bytes []byte) int32 {
	size := len(bytes)

	if uint(size) > h.freeSpace {
		fmt.Println("memory grown")
		numberOfPages := (uint(size) / wasmer.WasmPageSize) + 1
		grown := h.memory.Grow(wasmer.Pages(numberOfPages))
		if !grown {
			panic("couldn't grow memory")
		}
		h.freeSpace += (wasmer.WasmPageSize * numberOfPages)
	}

	ptr := h.nextPtrLocation

	memoryData := h.memory.Data()
	copy(memoryData[ptr:], bytes)

	h.nextPtrLocation += int32(size)
	h.freeSpace -= uint(size)

	return ptr
}

type AscPtr interface {
	ToPtr(heap *AscHeap) (int32, int32)
}

type AscReturnValue struct {
	name string
	ptr  int32
}

func NewAscReturnValue(name string) *AscReturnValue {
	return &AscReturnValue{
		name: name,
	}
}

func (v *AscReturnValue) ToPtr(heap *AscHeap) (int32, int32) {
	bs := make([]byte, 8)
	ptr := heap.Write(bs)
	v.ptr = ptr
	return ptr, int32(len(bs))
}

func (v *AscReturnValue) ReadData(env Environment) ([]byte, error) {
	//fmt.Printf("reading data for %s @ %d\n", v.name, v.ptr)
	ptr, err := env.ReadI32(v.ptr)
	if err != nil {
		return nil, fmt.Errorf("getting [%s] return value pointer: %w", v.name, err)

	}
	length, err := env.ReadI32(v.ptr + 4)
	if err != nil {
		return nil, fmt.Errorf("getting [%s] return value length: %w", v.name, err)
	}

	return env.ReadBytes(ptr, length)
}

type AscString string

func (h AscString) ToPtr(heap *AscHeap) (int32, int32) {
	bytes := []byte(h)
	return heap.Write(bytes), int32(len(bytes))
}

type AscBytes []byte

func (h AscBytes) ToPtr(heap *AscHeap) (int32, int32) {
	ptr := heap.Write(h)
	return ptr, int32(len(h))
}

func (r *Runtime) callFunction(heap *AscHeap, entrypoint *wasmer.Function, parameters []interface{}, returns []*AscReturnValue) (out interface{}, err error) {
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

	for _, returnValue := range returns {
		ptr, _ := returnValue.ToPtr(heap)
		fmt.Println("return pointer created @:", ptr)
		wasmParameters = append(wasmParameters, ptr)
	}

	//mem := r.env.GetMemory()
	out, err = entrypoint.Call(wasmParameters...)

	return
}

func (r *Runtime) getReturnPtrLength(valueLocation int32) (ptr int32, length int32, err error) {
	ptr, err = r.env.ReadI32(valueLocation)
	if err != nil {
		err = fmt.Errorf("getting return value pointer: %w", err)
		return
	}
	length, err = r.env.ReadI32(valueLocation + 4)
	if err != nil {
		err = fmt.Errorf("getting return value length: %w", err)
		return
	}
	return
}

func printMem(memory *wasmer.Memory) {
	data := memory.Data()
	for i, datum := range data {
		if i > 1024 {
			if datum == 0 {
				continue
			}
		}
		fmt.Print(datum, ", ")
	}
	println("")
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
