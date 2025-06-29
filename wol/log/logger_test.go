package wol_log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{LogLevel(999), "UNKNOWN"}, // Test invalid level
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultLoggerConfig(t *testing.T) {
	config := DefaultLoggerConfig()

	if config.Level != INFO {
		t.Errorf("DefaultLoggerConfig().Level = %v, want %v", config.Level, INFO)
	}

	if config.LogToFile != false {
		t.Errorf("DefaultLoggerConfig().LogToFile = %v, want false", config.LogToFile)
	}

	if config.LogToConsole != true {
		t.Errorf("DefaultLoggerConfig().LogToConsole = %v, want true", config.LogToConsole)
	}

	if config.LogFilePath != "" {
		t.Errorf("DefaultLoggerConfig().LogFilePath = %v, want empty string", config.LogFilePath)
	}
}

func TestNewLogger_ConsoleOnly(t *testing.T) {
	config := LoggerConfig{
		Level:        INFO,
		LogToFile:    false,
		LogToConsole: true,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v, want nil", err)
	}
	defer logger.Close()

	if logger.level != INFO {
		t.Errorf("Logger.level = %v, want %v", logger.level, INFO)
	}

	if logger.logFile != nil {
		t.Errorf("Logger.logFile should be nil for console-only logger")
	}
}

func TestNewLogger_FileOnly(t *testing.T) {
	// Create a temporary directory for test logs
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	config := LoggerConfig{
		Level:        DEBUG,
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v, want nil", err)
	}
	defer logger.Close()

	if logger.logFile == nil {
		t.Errorf("Logger.logFile should not be nil for file logger")
	}

	// Check that log file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logPath)
	}
}

func TestNewLogger_InvalidLogPath(t *testing.T) {
	// Create a scenario that should reliably fail across platforms
	tempDir := t.TempDir()

	// Create a regular file
	conflictingFile := filepath.Join(tempDir, "conflict")
	err := os.WriteFile(conflictingFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create conflicting file: %v", err)
	}

	// Try to create a log file "inside" this regular file (treating it as a directory)
	// This should fail because you can't create a directory inside a regular file
	invalidPath := filepath.Join(conflictingFile, "subdir", "test.log")

	config := LoggerConfig{
		Level:        INFO,
		LogToFile:    true,
		LogFilePath:  invalidPath,
		LogToConsole: false,
	}

	_, err = NewLogger(config)
	if err == nil {
		t.Errorf("NewLogger() expected error when trying to create directory inside a file, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLogger_LogLevels(t *testing.T) {
	// Create a temporary log file to capture output
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "level-test.log")

	config := LoggerConfig{
		Level:        WARN, // Set to WARN to test level filtering
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log messages at different levels
	logger.Debug("This debug message should not appear")
	logger.Info("This info message should not appear")
	logger.Warn("This warning message should appear")
	logger.Error("This error message should appear")

	logger.Close()

	// Read the log file and verify content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Check that only WARN and ERROR messages appear
	if strings.Contains(logContent, "debug message") {
		t.Errorf("Debug message should not appear in log when level is WARN")
	}

	if strings.Contains(logContent, "info message") {
		t.Errorf("Info message should not appear in log when level is WARN")
	}

	if !strings.Contains(logContent, "warning message") {
		t.Errorf("Warning message should appear in log when level is WARN")
	}

	if !strings.Contains(logContent, "error message") {
		t.Errorf("Error message should appear in log when level is WARN")
	}

	// Verify log format includes level tags
	if !strings.Contains(logContent, "[WARN]") {
		t.Errorf("Log should contain [WARN] tag")
	}

	if !strings.Contains(logContent, "[ERROR]") {
		t.Errorf("Log should contain [ERROR] tag")
	}
}

func TestLogger_LogWakeAttempt_Success(t *testing.T) {
	// Capture output for verification
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "wake-success.log")

	config := LoggerConfig{
		Level:        INFO,
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log a successful wake attempt
	testMAC := "AA:BB:CC:DD:EE:FF"
	testPort := 9
	logger.LogWakeAttempt(testMAC, testPort, true, nil)

	logger.Close()

	// Verify log content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	expectedParts := []string{
		"[INFO]",
		"Wake-on-LAN packet sent successfully",
		testMAC,
		fmt.Sprintf("port=%d", testPort),
	}

	for _, part := range expectedParts {
		if !strings.Contains(logContent, part) {
			t.Errorf("Log should contain %q, got: %s", part, logContent)
		}
	}
}

func TestLogger_LogWakeAttempt_Failure(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "wake-failure.log")

	config := LoggerConfig{
		Level:        ERROR,
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log a failed wake attempt
	testMAC := "BB:CC:DD:EE:FF:AA"
	testPort := 7
	testError := fmt.Errorf("network unreachable")
	logger.LogWakeAttempt(testMAC, testPort, false, testError)

	logger.Close()

	// Verify log content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	expectedParts := []string{
		"[ERROR]",
		"Failed to send Wake-on-LAN packet",
		testMAC,
		fmt.Sprintf("port=%d", testPort),
		"network unreachable",
	}

	for _, part := range expectedParts {
		if !strings.Contains(logContent, part) {
			t.Errorf("Log should contain %q, got: %s", part, logContent)
		}
	}
}

func TestLogger_LogPacketDetails(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "packet-details.log")

	config := LoggerConfig{
		Level:        DEBUG, // Need DEBUG level to see packet details
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log packet details
	testMAC := "CC:DD:EE:FF:AA:BB"
	testPacketSize := 102
	testPort := 9
	logger.LogPacketDetails(testMAC, testPacketSize, testPort)

	logger.Close()

	// Verify log content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	expectedParts := []string{
		"[DEBUG]",
		"Created magic packet",
		testMAC,
		fmt.Sprintf("Size=%d bytes", testPacketSize),
		fmt.Sprintf("Target=255.255.255.255:%d", testPort),
	}

	for _, part := range expectedParts {
		if !strings.Contains(logContent, part) {
			t.Errorf("Log should contain %q, got: %s", part, logContent)
		}
	}
}

func TestLogger_Close(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "close-test.log")

	config := LoggerConfig{
		Level:        INFO,
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Write something to the log
	logger.Info("Test message")

	// Close the logger
	err = logger.Close()
	if err != nil {
		t.Errorf("Logger.Close() error = %v, want nil", err)
	}

	// Verify we can read the file (indicating it was properly closed)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Errorf("Failed to read log file after close: %v", err)
	}

	if !strings.Contains(string(content), "Test message") {
		t.Errorf("Log file should contain test message")
	}
}

func TestLogger_MultipleLogs(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "multiple-logs.log")

	config := LoggerConfig{
		Level:        DEBUG,
		LogToFile:    true,
		LogFilePath:  logPath,
		LogToConsole: false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	// Log multiple messages
	logger.Debug("Debug message 1")
	logger.Info("Info message 1")
	logger.Warn("Warning message 1")
	logger.Error("Error message 1")

	logger.Debug("Debug message 2")
	logger.Info("Info message 2")

	// Close to flush buffers
	logger.Close()

	// Read and verify all messages are present
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	lines := strings.Split(strings.TrimSpace(logContent), "\n")

	if len(lines) != 6 {
		t.Errorf("Expected 6 log lines, got %d", len(lines))
	}

	// Verify order and content
	expectedMessages := []string{
		"Debug message 1",
		"Info message 1",
		"Warning message 1",
		"Error message 1",
		"Debug message 2",
		"Info message 2",
	}

	for i, expected := range expectedMessages {
		if i >= len(lines) || !strings.Contains(lines[i], expected) {
			t.Errorf("Line %d should contain %q, got: %s", i, expected, lines[i])
		}
	}
}
