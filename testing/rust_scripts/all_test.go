package assembly_scripts

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/streamingfast/wasm-runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			wasmFile:     "/Users/cbillett/devel/sf/wasm-runtime/testing/rust_scripts/hello/target/wasm32-unknown-unknown/release/hello_wasm.wasm",
			functionName: "hello",
			parameters:   []interface{}{"Charles"},
			expected:     "Hello Charles",
		},
	}

	for _, test := range tests {
		t.Run(test.wasmFile, func(t *testing.T) {
			recorder := &callRecorder{}
			env := wasm.RustEnvironment{CallRecorder: recorder}
			var returns reflect.Type
			if test.expected != nil {
				returns = reflect.TypeOf(test.expected)
			}

			//memoryAllocationFactory := func(instance *wasmer.Instance) wasmer.NativeFunction {
			//	function, err := instance.Exports.GetFunction("memory.allocate")
			//	if err != nil {
			//		panic(fmt.Errorf("getting memory.allocate func: %w", err))
			//	}
			//	return function
			//}

			//runtime := wasm.NewRuntime(env, wasm.WithMemoryAllocationFactory(memoryAllocationFactory))
			runtime := wasm.NewRuntime(&env, wasm.WithParameterPointSize())

			actual, err := runtime.Execute(test.wasmFile, test.functionName, returns, test.parameters...)

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
