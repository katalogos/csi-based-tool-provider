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

package toolprovider

import (
	"fmt"

	"github.com/golang/glog"
	"golang.org/x/net/context"
)

type logLevel int

const (
	levelVolumePublishing logLevel = 1
	levelImageManager logLevel = 2
	levelContainerCleaner logLevel = 2
	levelRunCommand logLevel = 3
	levelKubernetesMounts logLevel = 4
	levelBadger logLevel = 6
	levelBadgerDebug logLevel = 7
	levelGRPCCalls logLevel = 8
)


// TODO: Also add an environment variable to define if images should be pulled from inside or not.
// Or this should still be defined by the catalog


type loggerContextKeyType string

const (
	loggerKey loggerContextKeyType = "logger"
)

type logger interface {
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

var _ logger = &internalLogger{}

func contextLogger(ctx context.Context) logger {
	loggerValue := ctx.Value(loggerKey)
	logger, isLogger := loggerValue.(logger)
	if isLogger && logger != nil {
		return logger
	}
	return buildLogger("", 0)
}

func buildLogger(prefix string, level logLevel) logger {
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
