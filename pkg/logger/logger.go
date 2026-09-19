// Package logger 封装 zap 日志构造逻辑。
package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 根据 level 与 encoding 创建 zap.Logger。
// encoding 为 "json" 时使用 JSON 编码，否则使用 console。
// level 支持 debug/info/warn/error。
// 日志输出到 stdout，避免生产配置默认写到 stderr 导致看不到日志。
func New(level, encoding string) (*zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}

	ec := zap.NewProductionEncoderConfig()
	ec.EncodeTime = zapcore.ISO8601TimeEncoder // 人类可读时间
	if encoding != "json" {
		ec.EncodeLevel = zapcore.CapitalLevelEncoder // DEBUG/INFO/WARN/ERROR
	}

	var encoder zapcore.Encoder
	if encoding == "json" {
		encoder = zapcore.NewJSONEncoder(ec)
	} else {
		encoder = zapcore.NewConsoleEncoder(ec)
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), lvl)
	return zap.New(core, zap.AddCaller()), nil
}
