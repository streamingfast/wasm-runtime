package assembly_scripts

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/streamingfast/wasm-runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wasmerio/wasmer-go/wasmer"
)

func TestAssemblyScript(t *testing.T) {
	tests := []struct {
		wasmFile      string
		functionName  string
		parameters    []interface{}
		expectedCalls []call
		expected      interface{}
		expectedErr   error
	}{
		{
			wasmFile:     "scripts/i64.wasm",
			functionName: "main",
			parameters:   []interface{}{int64(-10)},
			expected:     int64(-20),
		},

		{
			// A u64 types must still be passed as a int64 value and is returned as a int64
			wasmFile:     "scripts/u64.wasm",
			functionName: "main",
			parameters:   []interface{}{int64(10)},
			expected:     int64(20),
		},

		{
			wasmFile:     "scripts/string.wasm",
			functionName: "main",
			parameters:   []interface{}{"some value"},
			expected:     "some ",
		},

		{
			wasmFile:     "scripts/uint8_array.wasm",
			functionName: "main",
			parameters:   []interface{}{[]byte{0xFA, 0xE9, 0xF1}},
			expected:     []byte{0xE6, 0xF5, 0xAF},
		},

		{
			wasmFile:      "imports/log_error.wasm",
			functionName:  "main",
			expectedCalls: []call{{"index", "log.log", []interface{}{int32(1), "log error abc - 123"}, nil}},
		},
		{
			wasmFile:     "scripts/hello.wasm",
			functionName: "hello",
			parameters:   []interface{}{"Colin"},
			expected:     "hello Colin",
		},
	}

	for _, test := range tests {
		t.Run(test.wasmFile, func(t *testing.T) {
			recorder := &callRecorder{}
			env := wasm.RustEnvironment{CallRecorder: recorder}
			var returns reflect.Type
			// FIXME
			//if test.expected != nil {
			//	returns = reflect.TypeOf(test.expected)
			//}

			memoryAllocationFactory := func(instance *wasmer.Instance) wasmer.NativeFunction {
				function, err := instance.Exports.GetFunction("memory.allocate")
				if err != nil {
					panic(fmt.Errorf("getting memory.allocate func: %w", err))
				}
				return function
			}

			runtime := wasm.NewRuntime(&env, wasm.WithMemoryAllocationFactory(memoryAllocationFactory))

			actual, err := runtime.Execute(filepath.Join("build", test.wasmFile), test.functionName, returns, test.parameters...)

			if test.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, test.expected, actual)

				if len(test.expectedCalls) > 0 {
					assert.Equal(t, test.expectedCalls, recorder.calls)
				}
			} else {
				assert.Equal(t, test.expectedErr, err)
			}
		})
	}
}

type call struct {
	module   string
	function string
	params   []interface{}
	returns  interface{}
}

type callRecorder struct {
	calls []call
}

func (r *callRecorder) Record(module, function string, params []interface{}, returns interface{}) {
	r.calls = append(r.calls, call{module, function, params, returns})
}

func (r *callRecorder) String() string {
	if len(r.calls) <= 0 {
		return "<empty>"
	}

	values := make([]string, len(r.calls))
	for i, call := range r.calls {
		values[i] = fmt.Sprintf("%s/%s %v %v", call.module, call.function, call.params, call.returns)
	}

	return strings.Join(values, ",")
}
