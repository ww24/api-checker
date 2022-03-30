package logger

import (
	"github.com/blendle/zapdriver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// core implements zapcore.Core.
type core struct {
	zapcore.Core
}

func newCore(c zapcore.Core) *core {
	return &core{
		Core: c,
	}
}

func (c *core) With(fields []zap.Field) zapcore.Core {
	return newCore(c.Core.With(fields))
}

//nolint: gocritic
func (c *core) Write(e zapcore.Entry, fields []zapcore.Field) error {
	if zapcore.ErrorLevel.Enabled(e.Level) {
		field := zapdriver.ErrorReport(e.Caller.PC, e.Caller.File, e.Caller.Line, true)
		fields = append(fields, field)
	}

	//nolint: wrapcheck
	return c.Core.Write(e, fields)
}

// see: https://cloud.google.com/error-reporting/reference/rest/v1beta1/ServiceContext
// see: https://cloud.google.com/error-reporting/docs/formatting-error-messages
type serviceContext struct {
	service string
	version string
}

func newServiceContext(service, version string) zap.Field {
	sc := &serviceContext{
		service: service,
		version: version,
	}
	return zap.Object("serviceContext", sc)
}

func (c *serviceContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("service", c.service)
	if c.version != "" {
		enc.AddString("version", c.version)
	}
	return nil
}
