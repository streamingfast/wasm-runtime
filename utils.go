package wasm

import (
	"fmt"
	"strings"

	"github.com/wasmerio/wasmer-go/wasmer"
)

type functionDefinition wasmer.Function

func (d *functionDefinition) String() string {
	return d.string("<function>")
}

func (d *functionDefinition) string(name string) string {
	f := (*wasmer.Function)(d)

	params := make([]string, 0, int(f.ParameterArity()))
	for _, param := range f.Type().Params() {
		params = append(params, param.Kind().String())
	}

	if f.ResultArity() <= 0 {
		return fmt.Sprintf("%s(%s)", name, strings.Join(params, ", "))
	}

	results := make([]string, 0, int(f.ResultArity()))
	for _, result := range f.Type().Results() {
		results = append(results, result.Kind().String())
	}

	return fmt.Sprintf("%s(%s) (%s)", name, strings.Join(params, ", "), strings.Join(results, ", "))
}

type namedFunctionDefinition struct {
	name     string
	function *wasmer.Function
}

func (d namedFunctionDefinition) String() string {
	return (*functionDefinition)(d.function).string(d.name)
}
