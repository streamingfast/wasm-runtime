package wasm

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
	"unicode/utf16"
)

type Environment interface {
	SetMemory(memory *wasmer.Memory)

	ReadBytes(offset int32) ([]byte, error)
	ReadString(offset int32, len int32) (string, error)

	// We should probably have a ReadSlice receiving some reflect. Type to deal with all the possibilities
	ReadI32s(offset int32) ([]int32, error)
	ReadStrings(offset int32, len int32) ([]string, error)

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

type RustEnvironment struct {
	d DefaultEnvironment
}

func (e *RustEnvironment) SetMemory(memory *wasmer.Memory) {
	e.d.SetMemory(memory)
}

func (e *RustEnvironment) ReadBytes(offset int32) ([]byte, error) {
	return e.d.ReadBytes(offset)
}

func (e *RustEnvironment) ReadString(offset int32, len int32) (string, error) {
	e.LogSegment("Data +size type?", offset-12, 16)

	bytes, err := e.d.segment(offset, len)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	if ztracer.Enabled() {
		zlog.Debug("read string content bytes", zap.Stringer("bytes", hexBytes(bytes)))
	}

	characters := make([]uint16, len)
	for i := int32(0); i < len; i++ {
		offset := i * 2
		characters[i] = uint16(bytes[offset+1])<<8 | uint16(bytes[offset])
	}

	return string(utf16.Decode(characters)), nil
}

func (e *RustEnvironment) ReadI32s(offset int32) ([]int32, error) {
	return e.d.ReadI32s(offset)
}

func (e *RustEnvironment) ReadStrings(offset int32, len int32) ([]string, error) {
	//TODO implement me
	panic("implement me")
}

func (e *RustEnvironment) LogSegment(message string, offset int32, length int32) {
	e.d.LogSegment(message, offset, length)
}

func (e *RustEnvironment) RecordCall(module, function string, params []interface{}, returns interface{}) {
	e.d.RecordCall(module, function, params, returns)
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

func (e *DefaultEnvironment) ReadString(offset int32, _ int32) (string, error) {
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

func (e *DefaultEnvironment) ReadStrings(offset int32, _ int32) (out []string, err error) {
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

		out[i], err = e.ReadString(stringOffset, 0)
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
