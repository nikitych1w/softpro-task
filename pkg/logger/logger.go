package logger

import (
	"github.com/nikitych1w/softpro-task/internal/config"
	"github.com/sirupsen/logrus"
)

func New(cfg *config.Config) *logrus.Logger {
	l := logrus.New()
	lvl, _ := logrus.ParseLevel(cfg.Log.Level)
	l.Level = lvl
	return l
}
