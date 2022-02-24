package wasm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
)

func newImports(runtimeEnv Environment, store *wasmer.Store) *wasmer.ImportObject {
	importObject := wasmer.NewImportObject()

	byModule := map[string][]impl{}
	for _, function := range functions {
		byModule[function.module] = append(byModule[function.module], function)
	}

	for module, impls := range byModule {
		namespace := map[string]wasmer.IntoExtern{}
		if module == "index" {
			// Necessary until all functions use the new format
			namespace = map[string]wasmer.IntoExtern{
				"bigDecimal.fromString":        wasmer.NewFunction(store, indexBigDecimalFromStringFunction, indexBigDecimalFromStringWASM),
				"typeConversion.stringToH160":  wasmer.NewFunction(store, indexTypeConversionStringToH160Function, indexTypeConversionStringToH160WASM),
				"store.get":                    wasmer.NewFunction(store, indexStoreGetFunction, indexStoreGetWASM),
				"store.set":                    wasmer.NewFunction(store, indexStoreSetFunction, indexStoreSetWASM),
				"ethereum.call":                wasmer.NewFunction(store, indexEthereumCallFunction, indexEthereumCallWASM),
				"typeConversion.bytesToString": wasmer.NewFunction(store, indexTypeConversionBytesToStringFunction, indexTypeConversionBytesToStringWASM),
				"dataSource.create":            wasmer.NewFunction(store, indexDataSourceCreateFunction, indexDataSourceCreateWASM),
			}
		}

		for _, i := range impls {
			impl := i
			function := impl.function
			if ztracer.Enabled() {
				function = func(env Environment, args []wasmer.Value) (out []wasmer.Value, err error) {
					name := impl.module + "/" + impl.name
					defer func() { zlog.Debug("terminated "+name+" returned "+valueSet(out).String(), zap.Error(err)) }()

					zlog.Debug("invoking " + name + valueSet(args).String())
					out, err = impl.function(env, args)
					return
				}
			}

			namespace[impl.name] = wasmer.NewFunctionWithEnvironment(store, impl.functionDef, runtimeEnv, func(env interface{}, args []wasmer.Value) ([]wasmer.Value, error) {
				return function(env.(Environment), args)
			})
		}

		importObject.Register(module, namespace)
	}

	return importObject
}

type impl struct {
	module      string
	name        string
	functionDef *wasmer.FunctionType
	function    implFunc
}

func intrinsics(module string, name string, params []*wasmer.ValueType, results []*wasmer.ValueType, f implFunc) impl {
	return impl{module, name, wasmer.NewFunctionType(params, results), f}
}

func (i impl) alias(module string, name string) impl {
	return impl{module, name, i.functionDef, i.function}
}

var functions = []impl{
	// Env module

	intrinsics(
		"env", "abort",
		params(wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
		returns(),
		func(env Environment, args []wasmer.Value) ([]wasmer.Value, error) {
			message, err := env.ReadString(args[0].I32(), 0) // FIXME
			if err != nil {
				return nil, fmt.Errorf("read message argument: %w", err)
			}

			filename, err := env.ReadString(args[1].I32(), 0) // FIXME
			if err != nil {
				return nil, fmt.Errorf("read filename argument: %w", err)
			}

			lineNumber := int(args[2].I32())
			columnNumber := int(args[3].I32())

			return nil, &abortError{message, filename, lineNumber, columnNumber}
		},
	),

	/// Index module

	intrinsics(
		"index", "typeConversion.bytesToHex",
		params(wasmer.I32),
		returns(wasmer.I32),
		func(env Environment, args []wasmer.Value) ([]wasmer.Value, error) {
			_, err := env.ReadBytes(args[0].I32())
			if err != nil {
				return nil, fmt.Errorf("read messages argument: %w", err)
			}

			return []wasmer.Value{wasmer.NewI32(0)}, nil
		},
	),

	intrinsics(
		"index", "log.log",
		params(wasmer.I32, wasmer.I32),
		returns(),
		func(env Environment, args []wasmer.Value) ([]wasmer.Value, error) {
			level := args[0].I32()
			message, err := env.ReadString(args[1].I32(), 0) // FIXME
			if err != nil {
				return nil, fmt.Errorf("read message argument: %w", err)
			}

			env.RecordCall("index", "log.log", []interface{}{level, message}, nil)
			return nil, nil
		},
	),

	intrinsics(
		"env", "println",
		params(wasmer.I32, wasmer.I32),
		returns(),
		func(env Environment, args []wasmer.Value) ([]wasmer.Value, error) {
			message, err := env.ReadString(args[0].I32(), args[1].I32())
			if err != nil {
				return nil, fmt.Errorf("read message argument: %w", err)
			}

			fmt.Println(message)

			return nil, nil
		},
	),
}

// Old way of doing things

var indexBigDecimalFromStringFunction = wasmer.NewFunctionType(params(wasmer.I32), returns(wasmer.I32))

func indexBigDecimalFromStringWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return []wasmer.Value{wasmer.NewI32(0)}, nil
}

var indexTypeConversionStringToH160Function = wasmer.NewFunctionType(params(wasmer.I32), returns(wasmer.I32))

func indexTypeConversionStringToH160WASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return []wasmer.Value{wasmer.NewI32(0)}, nil
}

var indexStoreGetFunction = wasmer.NewFunctionType(params(wasmer.I32, wasmer.I32), returns(wasmer.I32))

func indexStoreGetWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return []wasmer.Value{wasmer.NewI32(0)}, nil
}

var indexStoreSetFunction = wasmer.NewFunctionType(params(wasmer.I32, wasmer.I32, wasmer.I32), returns())

func indexStoreSetWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return nil, nil
}

var indexEthereumCallFunction = wasmer.NewFunctionType(params(wasmer.I32), returns(wasmer.I32))

func indexEthereumCallWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return []wasmer.Value{wasmer.NewI32(0)}, nil
}

var indexTypeConversionBytesToStringFunction = wasmer.NewFunctionType(params(wasmer.I32), returns(wasmer.I32))

func indexTypeConversionBytesToStringWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return []wasmer.Value{wasmer.NewI32(0)}, nil
}

var indexLogLogFunction = wasmer.NewFunctionType(params(wasmer.I32, wasmer.I32), returns())

func indexLogLogWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return nil, nil
}

var indexDataSourceCreateFunction = wasmer.NewFunctionType(params(wasmer.I32, wasmer.I32), returns())

func indexDataSourceCreateWASM(args []wasmer.Value) ([]wasmer.Value, error) {
	return nil, nil
}

// Helpers

func params(kinds ...wasmer.ValueKind) []*wasmer.ValueType {
	return wasmer.NewValueTypes(kinds...)
}

func returns(kinds ...wasmer.ValueKind) []*wasmer.ValueType {
	return wasmer.NewValueTypes(kinds...)
}

type implFunc func(env Environment, args []wasmer.Value) ([]wasmer.Value, error)

type valueSet []wasmer.Value

func (s valueSet) String() string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, fmt.Sprintf("%s (= %s)", v.Kind(), value(v)))
	}

	return fmt.Sprintf("(%s)", strings.Join(out, ", "))
}

type value wasmer.Value

func (v value) String() string {
	wasmValue := (wasmer.Value)(v)
	switch wasmValue.Kind() {
	case wasmer.I32:
		return strconv.FormatInt(int64(wasmValue.Unwrap().(int32)), 10)
	case wasmer.I64:
		return strconv.FormatInt(wasmValue.Unwrap().(int64), 10)
	case wasmer.F32:
		return strconv.FormatFloat(float64(wasmValue.Unwrap().(float32)), 'g', 16, 32)
	case wasmer.F64:
		return strconv.FormatFloat(wasmValue.Unwrap().(float64), 'g', 16, 64)
	}

	return "<ref>"
}
