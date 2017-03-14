package zapwriter

import (
	"bytes"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _testBuffer buffer

type buffer struct {
	bytes.Buffer
	mu sync.Mutex
}

func (b *buffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *buffer) Sync() error {
	return nil
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func (b *buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Buffer.Reset()
}

func Test() func() {
	cfg := NewConfig()
	encoder, _, _ := cfg.encoder()

	logger := zap.New(
		zapcore.NewCore(
			encoder,
			&_testBuffer,
			zapcore.DebugLevel,
		),
	)

	m := &manager{
		writers: make(map[string]WriteSyncer),
		cores:   make(map[string][]zapcore.Core),
		loggers: make(map[string]*zap.Logger),
	}

	m.loggers[""] = logger

	_testBuffer.Reset()
	_mutex.Lock()
	prev := _manager
	_manager = m
	_mutex.Unlock()

	return func() {
		_mutex.Lock()
		_manager = prev
		_mutex.Unlock()
	}
}

func TestWritten() string {
	return _testBuffer.String()
}
