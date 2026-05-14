package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

var currentLevel = InfoLevel

func Init(levelName string) error {
	lvl, err := ParseLevel(levelName)
	if err != nil {
		return err
	}
	currentLevel = lvl
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("")
	return nil
}

func ParseLevel(name string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DEBUG":
		return DebugLevel, nil
	case "INFO", "":
		return InfoLevel, nil
	case "WARN", "WARNING":
		return WarnLevel, nil
	case "ERROR":
		return ErrorLevel, nil
	case "FATAL":
		return FatalLevel, nil
	default:
		return InfoLevel, fmt.Errorf("invalid log level: %s", name)
	}
}

func enabled(level Level) bool {
	return level >= currentLevel
}

func formatLog(prefix, msg string, args ...interface{}) string {
	if len(args) == 0 {
		return fmt.Sprintf("[%s] %s", prefix, msg)
	}
	return fmt.Sprintf("[%s] %s", prefix, fmt.Sprintf(msg, args...))
}

func output(calldepth int, level Level, prefix, msg string, args ...interface{}) {
	if !enabled(level) {
		return
	}
	log.Output(calldepth, formatLog(prefix, msg, args...))
}

func Debugf(msg string, args ...interface{}) {
	output(3, DebugLevel, "DEBUG", msg, args...)
}

func Debug(msg string) {
	Debugf("%s", msg)
}

func Infof(msg string, args ...interface{}) {
	output(3, InfoLevel, "INFO", msg, args...)
}

func Info(msg string) {
	Infof("%s", msg)
}

func Warnf(msg string, args ...interface{}) {
	output(3, WarnLevel, "WARN", msg, args...)
}

func Warn(msg string) {
	Warnf("%s", msg)
}

func Errorf(msg string, args ...interface{}) {
	output(3, ErrorLevel, "ERROR", msg, args...)
}

func Error(msg string) {
	Errorf("%s", msg)
}

func Fatalf(msg string, args ...interface{}) {
	output(3, FatalLevel, "FATAL", msg, args...)
	os.Exit(1)
}

func Fatal(msg string) {
	Fatalf("%s", msg)
}
