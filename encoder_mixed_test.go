package zapwriter

import (
	"bytes"
	"fmt"
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

	if buf.String()[26:] != "] E test {\"key\":\"value\"}\n" {
		fmt.Println(buf.String())
		t.FailNow()
	}
}
