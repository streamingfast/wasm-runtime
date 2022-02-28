package assembly_scripts

import (
	"fmt"
	"strings"
	"testing"

	"github.com/streamingfast/wasm-runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustScript(t *testing.T) {
	tests := []struct {
		wasmFile            string
		functionName        string
		parameters          []interface{}
		outputsPtr          []*wasm.AscReturnValue
		expectedCalls       []call
		expectedReturnValue interface{}
		expectedErr         error
	}{
		{
			wasmFile:     "./hello/target/wasm32-unknown-unknown/release/hello_wasm.wasm",
			functionName: "hello",
			parameters:   []interface{}{"Charles"},
			outputsPtr: []*wasm.AscReturnValue{
				wasm.NewAscReturnValue("test.1"),
				wasm.NewAscReturnValue("test.2"),
			},
			expectedReturnValue: int32(42),
		},
		{
			wasmFile:     "./big_bytes/target/wasm32-unknown-unknown/release/big_bytes_wasm.wasm",
			functionName: "read_big_bytes",
			parameters:   []interface{}{createBytesArray(1)}, // max is 1087, anything above will break
			outputsPtr: []*wasm.AscReturnValue{
				wasm.NewAscReturnValue("test.1"),
			},
			expectedReturnValue: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.wasmFile, func(t *testing.T) {
			recorder := &callRecorder{}
			env := &wasm.RustEnvironment{
				CallRecorder: recorder,
			}
			runtime := wasm.NewRuntime(env, wasm.WithParameterPointSize())

			actual, err := runtime.Execute(test.wasmFile, test.functionName, test.parameters, test.outputsPtr...)
			require.NoError(t, err)

			for _, returnValue := range test.outputsPtr {
				data, err := returnValue.ReadData(env)
				require.NoError(t, err)
				fmt.Println("received data as string:", string(data))

			}

			if test.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, test.expectedReturnValue, actual)

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
	out := make([]byte, size*1024)
	out[0] = 1
	return out
}
