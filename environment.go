package wasm

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"unicode/utf16"

	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
)

var encoding = binary.LittleEndian

type Environment interface {
	SetMemory(memory *wasmer.Memory)
	GetMemory() *wasmer.Memory

	ReadBytes(offset int32, len int32) ([]byte, error)
	ReadString(offset int32, len int32) (string, error)

	ReadI32(ptr int32) (int32, error)

	LogSegment(message string, offset int32, length int32)
	RecordCall(module, function string, params []interface{}, returns interface{})
}

type CallRecorder interface {
	Record(module, name string, params []interface{}, returnValue interface{})
}

type RustEnvironment struct {
	CallRecorder CallRecorder
	memory       *wasmer.Memory
}

func (e *RustEnvironment) SetMemory(memory *wasmer.Memory) {
	e.memory = memory
}

func (e *RustEnvironment) GetMemory() *wasmer.Memory {
	return e.memory
}

func (e *RustEnvironment) ReadBytes(offset int32, length int32) ([]byte, error) {
	return e.segment(offset, length)
}

func (e *RustEnvironment) ReadString(offset int32, len int32) (string, error) {
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

func (e *RustEnvironment) ReadI32(offset int32) (int32, error) {
	return e.readI32("", offset)
}

func (e *RustEnvironment) LogSegment(message string, offset int32, length int32) {
	// a82700000000000003000000000000000300000000000000
	// e6f5af

	// a8270000 00000000 03000000   000000000300000000000000
	// e6f5af

	bytes, err := e.segment(offset, length)
	if err != nil {
		zlog.Info("unable to obtain data segment %env to %env for "+message, zap.Error(err))
	} else {
		zlog.Info(message, zap.Stringer("bytes", hexBytes(bytes)))
	}
}

func (e *RustEnvironment) RecordCall(module, function string, params []interface{}, returns interface{}) {
	if e.CallRecorder != nil {
		e.CallRecorder.Record(module, function, params, returns)
	}
}

func (e *RustEnvironment) Debug() string {
	if e.memory == nil {
		return "<empty>"
	}

	return hex.EncodeToString(e.memory.Data())
}

func (e *RustEnvironment) readI32(tag string, offset int32) (int32, error) {
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

func (e *RustEnvironment) dataAt(offset int32) ([]byte, error) {
	bytes := e.memory.Data()
	if offset < 0 {
		return nil, fmt.Errorf("offset %d env must be positive", offset)
	}

	if offset > int32(len(bytes)) {
		return nil, fmt.Errorf("offset %d env out of memory bounds ending at %d env", offset, len(bytes))
	}

	return e.memory.Data()[offset:], nil
}

func (e *RustEnvironment) segment(offset int32, length int32) ([]byte, error) {
	bytes := e.memory.Data()
	if offset < 0 {
		return nil, fmt.Errorf("offset %d env must be positive", offset)
	}

	if offset >= int32(len(bytes)) {
		return nil, fmt.Errorf("offset %d env out of memory bounds ending at %d env", offset, len(bytes))
	}

	end := offset + length
	if end > int32(len(bytes)) {
		return nil, fmt.Errorf("end %d env out of memory bounds ending at %d env", end, len(bytes))
	}

	return bytes[offset : offset+length], nil
}

func (e *RustEnvironment) readU32(tag string, offset int32) (uint32, error) {
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

func (e *RustEnvironment) readI64(tag string, offset int32) (int64, error) {
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
