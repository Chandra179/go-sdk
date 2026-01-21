package health

import (
	"context"

	"gosdk/pkg/logger"

	"github.com/stretchr/testify/mock"
)

type MockDBChecker struct {
	mock.Mock
}

func (m *MockDBChecker) PingContext(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockDBChecker) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockCacheChecker struct {
	mock.Mock
}

func (m *MockCacheChecker) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCacheChecker) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockKafkaChecker struct {
	mock.Mock
}

func (m *MockKafkaChecker) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockKafkaChecker) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockLogger struct{}

func (m *MockLogger) Info(ctx context.Context, msg string, fields ...logger.Field) {}

func (m *MockLogger) Error(ctx context.Context, msg string, fields ...logger.Field) {}

func (m *MockLogger) Debug(ctx context.Context, msg string, fields ...logger.Field) {}

func (m *MockLogger) Warn(ctx context.Context, msg string, fields ...logger.Field) {}

func (m *MockLogger) With(fields ...logger.Field) logger.Logger {
	return m
}
