/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"flag"
	"fmt"

	"context"

	"github.com/golang/glog"
	"github.com/spf13/viper"
)

type logLevel int

const (
	LevelVolumePublishing logLevel = 1
	LevelImageManager logLevel = 2
	LevelContainerCleaner logLevel = 2
	LevelRunCommand logLevel = 3
	LevelKubernetesMounts logLevel = 4
	LevelBadger logLevel = 6
	LevelBadgerDebug logLevel = 7
	LevelGRPCCalls logLevel = 8
)


type loggerContextKeyType string

const (
	loggerKey loggerContextKeyType = "logger"
)

type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	LeveledInfof(logLevel, string, ...interface{})
	Level(level logLevel)
}

type internalLogger struct {
	level glog.Level
	prefix string
}

var _ Logger = &internalLogger{}

func ContextLogger(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	logger, isLogger := loggerValue.(Logger)
	if isLogger && logger != nil {
		return logger
	}
	return BuildLogger("", 0)
}

func WithLogger(logger Logger) context.Context {
	return context.WithValue(
		context.Background(),
		loggerKey,
		logger)
}

func BuildLogger(prefix string, level logLevel) Logger {
	return &internalLogger {
		prefix: prefix,
		level: glog.Level(level),
	}
}

func (l *internalLogger) Errorf(format string, args ...interface{}) {
	glog.ErrorDepth(1, l.buildMessage(format, args...))
}

func (l *internalLogger) Warningf(format string, args ...interface{}) {
	glog.WarningDepth(1, l.buildMessage(format, args...))
}

func (l *internalLogger) Infof(format string, args ...interface{}) {
	if glog.V(glog.Level(l.level)) {
		glog.InfoDepth(1, l.buildMessage(format, args...))
	}
}

func (l *internalLogger) LeveledInfof(level logLevel, format string, args ...interface{}) {
	if glog.V(glog.Level(level)) {
		glog.InfoDepth(1, l.buildMessage(format, args...))
	}
}

func (l *internalLogger) buildMessage(format string, args ...interface{}) string {
	if l.prefix != "" {
		format = fmt.Sprintf("[%s] %s", l.prefix, format)
	}
	return fmt.Sprintf(format, args...)
}

func (l *internalLogger) Level(level logLevel) {
	l.level = glog.Level(level)
}

func SyncLogLevelFromViper() {
	verbose := viper.GetString("v")
	if verbose != "" {
		flag.Set("v", verbose)
	}
}
