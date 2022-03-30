package logger

import (
	"context"

	"github.com/blendle/zapdriver"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"
)

var defaultLogger *Logger

func InitializeLogger(ctx context.Context, name, version string) error {
	logger, err := New(ctx, name, version)
	if err != nil {
		return err
	}
	defaultLogger = logger
	return nil
}

func DefaultLogger(ctx context.Context) *zap.Logger {
	return defaultLogger.WithTraceFromContext(ctx)
}

type Logger struct {
	*zap.Logger
	projectID string
}

func NewNop() *Logger { return &Logger{Logger: zap.NewNop()} }

func New(ctx context.Context, name, version string) (*Logger, error) {
	opt := zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return newCore(core)
	})

	logger, err := zapdriver.NewProduction(opt)
	if err != nil {
		return nil, xerrors.Errorf("failed to initialize zapdriver: %w", err)
	}

	logger = logger.With(newServiceContext(name, version))

	projectID, err := getProjectID(ctx)
	if err != nil {
		logger.Warn("failed to get project id", zap.Error(err))
	}

	return &Logger{
		Logger:    logger,
		projectID: projectID,
	}, nil
}

func (l *Logger) WithTraceFromContext(ctx context.Context) *zap.Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() || l.projectID == "" {
		return l.Logger
	}

	fields := zapdriver.TraceContext(
		spanCtx.TraceID().String(),
		spanCtx.SpanID().String(),
		spanCtx.IsSampled(),
		l.projectID,
	)
	return l.With(fields...)
}

func getProjectID(ctx context.Context) (string, error) {
	cred, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return "", xerrors.Errorf("failed to find default credentials: %w", err)
	}

	if cred.ProjectID == "" {
		return "", xerrors.Errorf("project ID not found")
	}

	return cred.ProjectID, nil
}
