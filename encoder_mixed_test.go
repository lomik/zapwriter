package zapwriter

import (
	"bytes"
	"testing"

	"github.com/uber-go/zap"
)

func TestMixedEncoder(t *testing.T) {
	var buf bytes.Buffer

	logger := zap.New(
		NewMixedEncoder(),
		zap.DebugLevel,
		zap.Output(zap.AddSync(&buf)),
	)

	logger.Error("test", zap.String("key", "value"))

	if buf.String()[20:] != "] E test {\"key\":\"value\"}\n" {
		t.FailNow()
	}
}
