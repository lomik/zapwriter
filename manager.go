package zapwriter

import (
	"fmt"
	"net/url"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Manager interface {
	Default() *zap.Logger
	Logger(logger string) *zap.Logger
}

type manager struct {
	writers map[string]WriteSyncer
	loggers map[string][]*zap.Logger
	final   map[string]*zap.Logger // tee to loggers
}

func NewManager(conf []*Config) (Manager, error) {
	return makeManager(conf, false, nil)
}

func CheckForManager(conf []*Config, allowNames []string) error {
	_, err := makeManager(conf, true, allowNames)
	return err
}

func (m *manager) Default() *zap.Logger {
	if logger, ok := m.final[""]; ok {
		return logger
	}
	return zap.NewNop()
}

func (m *manager) Logger(logger string) *zap.Logger {
	if logger, ok := m.final[logger]; ok {
		return logger
	}
	return m.Default()
}

func makeManager(conf []*Config, checkOnly bool, allowNames []string) (Manager, error) {
	// check names
	if allowNames != nil {
		namesMap := make(map[string]bool)
		namesMap[""] = true
		for _, s := range allowNames {
			namesMap[s] = true
		}

		for _, cfg := range conf {
			if !namesMap[cfg.Logger] {
				return nil, fmt.Errorf("unknown logger name %#v", cfg.Logger)
			}
		}
	}

	// check config params
	for _, cfg := range conf {
		_, _, err := cfg.encoder()
		if err != nil {
			return nil, err
		}
	}

	// check complete
	if checkOnly {
		return nil, nil
	}

	// create writers
	m := &manager{
		writers: make(map[string]WriteSyncer),
		loggers: make(map[string][]*zap.Logger),
		final:   make(map[string]*zap.Logger),
	}

	for _, cfg := range conf {
		u, err := url.Parse(cfg.File)
		if err != nil {
			return nil, err
		}

		if _, ok := m.loggers[cfg.Logger]; !ok {
			m.loggers[cfg.Logger] = make([]*zap.Logger, 0)
		}

		if strings.ToLower(u.Path) == "none" {
			m.loggers[cfg.Logger] = append(m.loggers[cfg.Logger], zap.NewNop())
			continue
		}

		encoder, atomicLevel, err := cfg.encoder()
		if err != nil {
			return nil, err
		}

		ws, ok := m.writers[u.Path]
		if !ok {
			ws, err = New(cfg.File)
			if err != nil {
				return nil, err
			}
			m.writers[u.Path] = ws
		}

		m.loggers[cfg.Logger] = append(m.loggers[cfg.Logger],
			zap.New(zapcore.NewCore(encoder, ws, atomicLevel)),
		)
	}

	return m, nil
}
