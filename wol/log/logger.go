package wol_log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type Logger struct {
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	level       LogLevel
	logFile     *os.File
}

type LoggerConfig struct {
	Level        LogLevel
	LogToFile    bool
	LogFilePath  string
	LogToConsole bool
}

func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:        INFO,
		LogToFile:    false,
		LogFilePath:  "",
		LogToConsole: true,
	}
}

func NewLogger(config LoggerConfig) (*Logger, error) {
	logger := &Logger{
		level: config.Level,
	}

	var writers []io.Writer

	if config.LogToConsole {
		writers = append(writers, os.Stdout)
	}

	if config.LogToFile {
		if config.LogFilePath == "" {
			config.LogFilePath = getDefaultLogPath()
		}

		logDir := filepath.Dir(config.LogFilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
		}

		logFile, err := os.OpenFile(config.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", config.LogFilePath, err)
		}

		logger.logFile = logFile

		writers = append(writers, logFile)
	}

	multiWriter := io.MultiWriter(writers...)

	flags := log.Ldate | log.Ltime | log.Lmicroseconds

	logger.debugLogger = log.New(multiWriter, "[DEBUG] ", flags)
	logger.infoLogger = log.New(multiWriter, "[INFO] ", flags)
	logger.warnLogger = log.New(multiWriter, "[WARN] ", flags)
	logger.errorLogger = log.New(multiWriter, "[ERROR] ", flags)

	return logger, nil
}

func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}

	return nil
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.debugLogger.Printf(format, args...)
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.infoLogger.Printf(format, args...)
	}
}

func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.warnLogger.Printf(format, args...)
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.errorLogger.Printf(format, args...)
	}
}

func (l *Logger) LogWakeAttempt(mac string, port int, success bool, err error) {
	if success {
		l.Info("Wake-on-LAN packet sent successfully to MAC=%s on port=%d", mac, port)
	} else {
		l.Error("Failed to send Wake-on-LAN packet to MAC=%s on port=%d: %v", mac, port, err)
	}
}

func (l *Logger) LogPacketDetails(mac string, packetSize int, port int) {
	l.Debug("Created magic packet: MAC=%s, Size=%d bytes, Target=255.255.255.255:%d", mac, packetSize, port)
}

func getDefaultLogPath() string {
	timestamp := time.Now().Format("2006-01-02")
	return fmt.Sprintf("wol-server-%s.log", timestamp)
}
