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
	writers map[string]WriteSyncer    // path -> writer
	cores   map[string][]zapcore.Core // logger name -> cores
	loggers map[string]*zap.Logger    // logger name -> logger
}

func NewManager(conf []*Config) (Manager, error) {
	return makeManager(conf, false, nil)
}

func CheckForManager(conf []*Config, allowNames []string) error {
	_, err := makeManager(conf, true, allowNames)
	return err
}

func (m *manager) Default() *zap.Logger {
	if logger, ok := m.loggers[""]; ok {
		return logger
	}
	return zap.NewNop()
}

func (m *manager) Logger(logger string) *zap.Logger {
	if logger, ok := m.loggers[logger]; ok {
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

	m := &manager{
		writers: make(map[string]WriteSyncer),
		cores:   make(map[string][]zapcore.Core),
		loggers: make(map[string]*zap.Logger),
	}

	// create writers and cores
	for _, cfg := range conf {
		u, err := url.Parse(cfg.File)
		if err != nil {
			return nil, err
		}

		if _, ok := m.loggers[cfg.Logger]; !ok {
			m.cores[cfg.Logger] = make([]zapcore.Core, 0)
		}

		if strings.ToLower(u.Path) == "none" {
			m.cores[cfg.Logger] = append(m.cores[cfg.Logger], zapcore.NewNopCore())
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

		m.cores[cfg.Logger] = append(m.cores[cfg.Logger], zapcore.NewCore(encoder, ws, atomicLevel))
	}

	// make loggers
	for k, cores := range m.cores {
		m.loggers[k] = zap.New(zapcore.NewTee(cores...))
	}

	return m, nil
}
