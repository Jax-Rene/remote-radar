package notifier

import (
	"context"
	"log"
	"os"

	"remote-radar/internal/model"
)

// LogNotifier 仅打印新增职位，适合开发阶段使用。
type LogNotifier struct {
	logger *log.Logger
}

// NewLogNotifier 创建日志通知器，未提供 logger 时默认输出到标准输出。
func NewLogNotifier(logger *log.Logger) *LogNotifier {
	if logger == nil {
		logger = log.New(os.Stdout, "[notify] ", log.LstdFlags)
	}
	return &LogNotifier{logger: logger}
}

// Notify 逐条打印新增职位信息。
func (n LogNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		n.logger.Printf("new job: %s (%s) %s", job.Title, job.Source, job.URL)
	}
	return nil
}
