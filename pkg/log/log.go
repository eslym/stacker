package log

import (
	"fmt"
	"io"
	"os"
	"time"
)

func Printf(module string, format string, a ...any) {
	if module != "" {
		module = "[" + module + "] "
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("[%s] %s%s\n", timestamp, module, fmt.Sprintf(format, a...))
	_, _ = io.WriteString(os.Stdout, message)
}

func Errorf(module string, format string, a ...any) {
	if module != "" {
		module = "[" + module + "] "
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("[%s] %s%s\n", timestamp, module, fmt.Sprintf(format, a...))
	_, _ = io.WriteString(os.Stderr, message)
}
