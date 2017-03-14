package zapwriter

import (
	"strings"
	"testing"
)

func TestTesting(t *testing.T) {
	defer Test()()

	Default().Info("info message")

	if !strings.Contains(TestWritten(), "info message") {
		t.FailNow()
	}
}
