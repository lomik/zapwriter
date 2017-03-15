// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package zapwriter

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// For JSON-escaping; see jsonEncoder.safeAddString below.
const _hex = "0123456789abcdef"

var _mixedPool = sync.Pool{New: func() interface{} {
	return &mixedEncoder{}
}}

var _mixedBufferPool = buffer.NewPool()

func getMixedEncoder() *mixedEncoder {
	return _mixedPool.Get().(*mixedEncoder)
}

func putMixedEncoder(enc *mixedEncoder) {
	enc.EncoderConfig = nil
	enc.buf = nil
	enc.spaced = false
	enc.openNamespaces = 0
	_mixedPool.Put(enc)
}

type mixedEncoder struct {
	*zapcore.EncoderConfig
	buf            *buffer.Buffer
	spaced         bool // include spaces after colons and commas
	openNamespaces int
}

// NewJSONEncoder creates a fast, low-allocation JSON encoder. The encoder
// appropriately escapes all field keys and values.
//
// Note that the encoder doesn't deduplicate keys, so it's possible to produce
// a message like
//   {"foo":"bar","foo":"baz"}
// This is permitted by the JSON specification, but not encouraged. Many
// libraries will ignore duplicate key-value pairs (typically keeping the last
// pair) when unmarshaling, but users should attempt to avoid adding duplicate
// keys.
func NewMixedEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return newMixedEncoder(cfg, false)
}

func newMixedEncoder(cfg zapcore.EncoderConfig, spaced bool) *mixedEncoder {
	return &mixedEncoder{
		EncoderConfig: &cfg,
		buf:           _mixedBufferPool.Get(),
		spaced:        spaced,
	}
}

func (enc *mixedEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc.addKey(key)
	return enc.AppendArray(arr)
}

func (enc *mixedEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	enc.addKey(key)
	return enc.AppendObject(obj)
}

func (enc *mixedEncoder) AddBinary(key string, val []byte) {
	enc.AddString(key, base64.StdEncoding.EncodeToString(val))
}

func (enc *mixedEncoder) AddByteString(key string, val []byte) {
	enc.addKey(key)
	enc.AppendByteString(val)
}

func (enc *mixedEncoder) AddBool(key string, val bool) {
	enc.addKey(key)
	enc.AppendBool(val)
}

func (enc *mixedEncoder) AddComplex128(key string, val complex128) {
	enc.addKey(key)
	enc.AppendComplex128(val)
}

func (enc *mixedEncoder) AddDuration(key string, val time.Duration) {
	enc.addKey(key)
	enc.AppendDuration(val)
}

func (enc *mixedEncoder) AddFloat64(key string, val float64) {
	enc.addKey(key)
	enc.AppendFloat64(val)
}

func (enc *mixedEncoder) AddInt64(key string, val int64) {
	enc.addKey(key)
	enc.AppendInt64(val)
}

func (enc *mixedEncoder) AddReflected(key string, obj interface{}) error {
	marshaled, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	enc.addKey(key)
	_, err = enc.buf.Write(marshaled)
	return err
}

func (enc *mixedEncoder) OpenNamespace(key string) {
	enc.addKey(key)
	enc.buf.AppendByte('{')
	enc.openNamespaces++
}

func (enc *mixedEncoder) AddString(key, val string) {
	enc.addKey(key)
	enc.AppendString(val)
}

func (enc *mixedEncoder) AddTime(key string, val time.Time) {
	enc.addKey(key)
	enc.AppendTime(val)
}

func (enc *mixedEncoder) AddUint64(key string, val uint64) {
	enc.addKey(key)
	enc.AppendUint64(val)
}

func (enc *mixedEncoder) AppendArray(arr zapcore.ArrayMarshaler) error {
	enc.addElementSeparator()
	enc.buf.AppendByte('[')
	err := arr.MarshalLogArray(enc)
	enc.buf.AppendByte(']')
	return err
}

func (enc *mixedEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	enc.addElementSeparator()
	enc.buf.AppendByte('{')
	err := obj.MarshalLogObject(enc)
	enc.buf.AppendByte('}')
	return err
}

func (enc *mixedEncoder) AppendBool(val bool) {
	enc.addElementSeparator()
	enc.buf.AppendBool(val)
}

func (enc *mixedEncoder) AppendByteString(val []byte) {
	enc.addElementSeparator()
	enc.buf.AppendByte('"')
	enc.safeAddByteString(val)
	enc.buf.AppendByte('"')
}

func (enc *mixedEncoder) AppendComplex128(val complex128) {
	enc.addElementSeparator()
	// Cast to a platform-independent, fixed-size type.
	r, i := float64(real(val)), float64(imag(val))
	enc.buf.AppendByte('"')
	// Because we're always in a quoted string, we can use strconv without
	// special-casing NaN and +/-Inf.
	enc.buf.AppendFloat(r, 64)
	enc.buf.AppendByte('+')
	enc.buf.AppendFloat(i, 64)
	enc.buf.AppendByte('i')
	enc.buf.AppendByte('"')
}

func (enc *mixedEncoder) AppendDuration(val time.Duration) {
	cur := enc.buf.Len()
	enc.EncodeDuration(val, enc)
	if cur == enc.buf.Len() {
		// User-supplied EncodeDuration is a no-op. Fall back to nanoseconds to keep
		// JSON valid.
		enc.AppendInt64(int64(val))
	}
}

func (enc *mixedEncoder) AppendInt64(val int64) {
	enc.addElementSeparator()
	enc.buf.AppendInt(val)
}

func (enc *mixedEncoder) AppendReflected(val interface{}) error {
	marshaled, err := json.Marshal(val)
	if err != nil {
		return err
	}
	enc.addElementSeparator()
	_, err = enc.buf.Write(marshaled)
	return err
}

func (enc *mixedEncoder) AppendString(val string) {
	enc.addElementSeparator()
	enc.buf.AppendByte('"')
	enc.safeAddString(val)
	enc.buf.AppendByte('"')
}

func (enc *mixedEncoder) AppendTime(val time.Time) {
	cur := enc.buf.Len()
	enc.EncodeTime(val, enc)
	if cur == enc.buf.Len() {
		// User-supplied EncodeTime is a no-op. Fall back to nanos since epoch to keep
		// output JSON valid.
		enc.AppendInt64(val.UnixNano())
	}
}

func (enc *mixedEncoder) AppendUint64(val uint64) {
	enc.addElementSeparator()
	enc.buf.AppendUint(val)
}

func (enc *mixedEncoder) AddComplex64(k string, v complex64) { enc.AddComplex128(k, complex128(v)) }
func (enc *mixedEncoder) AddFloat32(k string, v float32)     { enc.AddFloat64(k, float64(v)) }
func (enc *mixedEncoder) AddInt(k string, v int)             { enc.AddInt64(k, int64(v)) }
func (enc *mixedEncoder) AddInt32(k string, v int32)         { enc.AddInt64(k, int64(v)) }
func (enc *mixedEncoder) AddInt16(k string, v int16)         { enc.AddInt64(k, int64(v)) }
func (enc *mixedEncoder) AddInt8(k string, v int8)           { enc.AddInt64(k, int64(v)) }
func (enc *mixedEncoder) AddUint(k string, v uint)           { enc.AddUint64(k, uint64(v)) }
func (enc *mixedEncoder) AddUint32(k string, v uint32)       { enc.AddUint64(k, uint64(v)) }
func (enc *mixedEncoder) AddUint16(k string, v uint16)       { enc.AddUint64(k, uint64(v)) }
func (enc *mixedEncoder) AddUint8(k string, v uint8)         { enc.AddUint64(k, uint64(v)) }
func (enc *mixedEncoder) AddUintptr(k string, v uintptr)     { enc.AddUint64(k, uint64(v)) }
func (enc *mixedEncoder) AppendComplex64(v complex64)        { enc.AppendComplex128(complex128(v)) }
func (enc *mixedEncoder) AppendFloat64(v float64)            { enc.appendFloat(v, 64) }
func (enc *mixedEncoder) AppendFloat32(v float32)            { enc.appendFloat(float64(v), 32) }
func (enc *mixedEncoder) AppendInt(v int)                    { enc.AppendInt64(int64(v)) }
func (enc *mixedEncoder) AppendInt32(v int32)                { enc.AppendInt64(int64(v)) }
func (enc *mixedEncoder) AppendInt16(v int16)                { enc.AppendInt64(int64(v)) }
func (enc *mixedEncoder) AppendInt8(v int8)                  { enc.AppendInt64(int64(v)) }
func (enc *mixedEncoder) AppendUint(v uint)                  { enc.AppendUint64(uint64(v)) }
func (enc *mixedEncoder) AppendUint32(v uint32)              { enc.AppendUint64(uint64(v)) }
func (enc *mixedEncoder) AppendUint16(v uint16)              { enc.AppendUint64(uint64(v)) }
func (enc *mixedEncoder) AppendUint8(v uint8)                { enc.AppendUint64(uint64(v)) }
func (enc *mixedEncoder) AppendUintptr(v uintptr)            { enc.AppendUint64(uint64(v)) }

func (enc *mixedEncoder) Clone() zapcore.Encoder {
	clone := enc.clone()
	clone.buf.Write(enc.buf.Bytes())
	return clone
}

func (enc *mixedEncoder) clone() *mixedEncoder {
	clone := getMixedEncoder()
	clone.EncoderConfig = enc.EncoderConfig
	clone.spaced = enc.spaced
	clone.openNamespaces = enc.openNamespaces
	clone.buf = _mixedBufferPool.Get()
	return clone
}

func addFields(enc zapcore.ObjectEncoder, fields []zapcore.Field) {
	for i := range fields {
		fields[i].AddTo(enc)
	}
}

func (enc *mixedEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	final := enc.clone()

	wr := getDirectWriteEncoder()
	wr.buf = final.buf

	if enc.TimeKey != "" && enc.EncodeTime != nil {
		final.buf.AppendByte('[')
		enc.EncodeTime(ent.Time, wr)
		final.buf.AppendByte(']')
	}

	if enc.LevelKey != "" && enc.EncodeLevel != nil {
		if final.buf.Len() > 0 {
			final.buf.AppendByte(' ')
		}
		enc.EncodeLevel(ent.Level, wr)
	}

	if ent.LoggerName != "" && enc.NameKey != "" {
		if final.buf.Len() > 0 {
			final.buf.AppendByte(' ')
		}
		final.buf.AppendByte('[')
		wr.AppendString(ent.LoggerName)
		final.buf.AppendByte(']')
	}

	if ent.Caller.Defined && enc.CallerKey != "" && enc.EncodeCaller != nil {
		if final.buf.Len() > 0 {
			final.buf.AppendByte(' ')
		}
		enc.EncodeCaller(ent.Caller, wr)
		final.buf.AppendByte(' ')
	}

	putDirectWriteEncoder(wr)

	// Add the message itself.
	if final.MessageKey != "" {
		if final.buf.Len() > 0 {
			final.buf.AppendByte(' ')
		}
		final.buf.AppendString(ent.Message)
	}

	if final.buf.Len() > 0 {
		final.buf.AppendByte(' ')
	}

	final.buf.AppendByte('{')

	if enc.buf.Len() > 0 {
		final.addElementSeparator()
		final.buf.Write(enc.buf.Bytes())
	}
	addFields(final, fields)
	final.closeOpenNamespaces()
	if ent.Stack != "" && final.StacktraceKey != "" {
		final.AddString(final.StacktraceKey, ent.Stack)
	}
	final.buf.AppendByte('}')
	final.buf.AppendByte('\n')

	ret := final.buf
	putMixedEncoder(final)
	return ret, nil
}

func (enc *mixedEncoder) truncate() {
	enc.buf.Reset()
}

func (enc *mixedEncoder) closeOpenNamespaces() {
	for i := 0; i < enc.openNamespaces; i++ {
		enc.buf.AppendByte('}')
	}
}

func (enc *mixedEncoder) addKey(key string) {
	enc.addElementSeparator()
	enc.buf.AppendByte('"')
	enc.safeAddString(key)
	enc.buf.AppendByte('"')
	enc.buf.AppendByte(':')
	if enc.spaced {
		enc.buf.AppendByte(' ')
	}
}

func (enc *mixedEncoder) addElementSeparator() {
	last := enc.buf.Len() - 1
	if last < 0 {
		return
	}
	switch enc.buf.Bytes()[last] {
	case '{', '[', ':', ',', ' ':
		return
	default:
		enc.buf.AppendByte(',')
		if enc.spaced {
			enc.buf.AppendByte(' ')
		}
	}
}

func (enc *mixedEncoder) appendFloat(val float64, bitSize int) {
	enc.addElementSeparator()
	switch {
	case math.IsNaN(val):
		enc.buf.AppendString(`"NaN"`)
	case math.IsInf(val, 1):
		enc.buf.AppendString(`"+Inf"`)
	case math.IsInf(val, -1):
		enc.buf.AppendString(`"-Inf"`)
	default:
		enc.buf.AppendFloat(val, bitSize)
	}
}

// safeAddString JSON-escapes a string and appends it to the internal buffer.
// Unlike the standard library's encoder, it doesn't attempt to protect the
// user from browser vulnerabilities or JSONP-related problems.
func (enc *mixedEncoder) safeAddString(s string) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.buf.AppendString(s[i : i+size])
		i += size
	}
}

// safeAddByteString is no-alloc equivalent of safeAddString(string(s)) for s []byte.
func (enc *mixedEncoder) safeAddByteString(s []byte) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.buf.Write(s[i : i+size])
		i += size
	}
}

// tryAddRuneSelf appends b if it is valid UTF-8 character represented in a single byte.
func (enc *mixedEncoder) tryAddRuneSelf(b byte) bool {
	if b >= utf8.RuneSelf {
		return false
	}
	if 0x20 <= b && b != '\\' && b != '"' {
		enc.buf.AppendByte(b)
		return true
	}
	switch b {
	case '\\', '"':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte(b)
	case '\n':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('n')
	case '\r':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('r')
	case '\t':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('t')
	default:
		// Encode bytes < 0x20, except for the escape sequences above.
		enc.buf.AppendString(`\u00`)
		enc.buf.AppendByte(_hex[b>>4])
		enc.buf.AppendByte(_hex[b&0xF])
	}
	return true
}

func (enc *mixedEncoder) tryAddRuneError(r rune, size int) bool {
	if r == utf8.RuneError && size == 1 {
		enc.buf.AppendString(`\ufffd`)
		return true
	}
	return false
}
