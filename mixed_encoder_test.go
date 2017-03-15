package zapwriter

import (
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func TestMixedEncoder(t *testing.T) {
	cfg := NewConfig()
	cfg.Encoding = "mixed"

	defer testWithConfig(cfg)()

	zap.L().Named("carbonserver").Info("message text", zap.String("key", "value"))

	fmt.Println(TestCapture())
}
