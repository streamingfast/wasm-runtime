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
			wasmFile:     "/Users/eduardvoiculescu/git/wasm-runtime/testing/rust_scripts/hello/target/wasm32-unknown-unknown/release/hello_wasm.wasm",
			functionName: "hello",
			parameters:   []interface{}{"Charles"},
			expected:     int32(42),
		},
		{
			wasmFile:     "/Users/eduardvoiculescu/git/wasm-runtime/testing/rust_scripts/bigBytes/target/wasm32-unknown-unknown/release/bigBytes_wasm.wasm",
			functionName: "read_big_bytes",
			parameters:   []interface{}{createBytesArray(1200)}, // max is 1087, anything above will break
			expected:     nil,
		},
	}

	for _, test := range tests {
		t.Run(test.wasmFile, func(t *testing.T) {
			recorder := &callRecorder{}
			env := &wasm.RustEnvironment{CallRecorder: recorder}
			var returns reflect.Type
			if test.expected != nil {
				returns = reflect.TypeOf(test.expected)
			}
			runtime := wasm.NewRuntime(env, wasm.WithParameterPointSize())

			ret := wasm.NewAscReturnValue("test.1")
			ret2 := wasm.NewAscReturnValue("test.2")

			actual, err := runtime.Execute(test.wasmFile, test.functionName, returns, test.parameters, ret)
			data, err := ret.ReadData(env)
			require.NoError(t, err)
			fmt.Println("received data as string:", string(data))
			data2, err := ret2.ReadData(env)
			require.NoError(t, err)
			fmt.Println("received data2 as string:", string(data2))

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

// 1 -> 1KB, 1 000 -> 1MB, 1 000 000 -> 1GB
func createBytesArray(size int32) []byte {
	return make([]byte, size*1024)
}
