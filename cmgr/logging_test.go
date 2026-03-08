package cmgr

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	for _, level := range []LogLevel{DISABLED, ERROR, WARN, INFO, DEBUG} {
		l := newLogger(level)
		if l == nil {
			t.Fatalf("newLogger(%d) returned nil", level)
		}
		if l.logLevel != level {
			t.Errorf("expected logLevel %d, got %d", level, l.logLevel)
		}
		if l.logger == nil {
			t.Error("expected non-nil underlying logger")
		}
	}
}

func TestLogLevelConstants(t *testing.T) {
	// Verify the ordering of log levels
	if DISABLED >= ERROR {
		t.Error("DISABLED should be less than ERROR")
	}
	if ERROR >= WARN {
		t.Error("ERROR should be less than WARN")
	}
	if WARN >= INFO {
		t.Error("WARN should be less than INFO")
	}
	if INFO >= DEBUG {
		t.Error("INFO should be less than DEBUG")
	}
}

func TestLoggerDebugFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := &logger{
		logger:   log.New(&buf, "cmgr: ", 0),
		logLevel: DISABLED,
	}

	// At DISABLED level, nothing should be logged
	l.debug("test message")
	if buf.Len() != 0 {
		t.Error("DISABLED level should not produce debug output")
	}

	l.logLevel = ERROR
	l.debug("test message")
	if buf.Len() != 0 {
		t.Error("ERROR level should not produce debug output")
	}

	l.logLevel = DEBUG
	l.debug("test message")
	if buf.Len() == 0 {
		t.Error("DEBUG level should produce debug output")
	}
	if !strings.Contains(buf.String(), "DEBUG:") {
		t.Errorf("expected DEBUG prefix, got %q", buf.String())
	}
}

func TestLoggerInfoFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := &logger{
		logger:   log.New(&buf, "cmgr: ", 0),
		logLevel: WARN,
	}

	l.info("test message")
	if buf.Len() != 0 {
		t.Error("WARN level should not produce info output")
	}

	l.logLevel = INFO
	l.info("test message")
	if buf.Len() == 0 {
		t.Error("INFO level should produce info output")
	}
	if !strings.Contains(buf.String(), "INFO:") {
		t.Errorf("expected INFO prefix, got %q", buf.String())
	}
}

func TestLoggerWarnFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := &logger{
		logger:   log.New(&buf, "cmgr: ", 0),
		logLevel: ERROR,
	}

	l.warn("test message")
	if buf.Len() != 0 {
		t.Error("ERROR level should not produce warn output")
	}

	l.logLevel = WARN
	l.warn("test message")
	if buf.Len() == 0 {
		t.Error("WARN level should produce warn output")
	}
	if !strings.Contains(buf.String(), "WARN:") {
		t.Errorf("expected WARN prefix, got %q", buf.String())
	}
}

func TestLoggerErrorFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := &logger{
		logger:   log.New(&buf, "cmgr: ", 0),
		logLevel: DISABLED,
	}

	l.error("test message")
	if buf.Len() != 0 {
		t.Error("DISABLED level should not produce error output")
	}

	l.logLevel = ERROR
	l.error("test message")
	if buf.Len() == 0 {
		t.Error("ERROR level should produce error output")
	}
	if !strings.Contains(buf.String(), "ERROR:") {
		t.Errorf("expected ERROR prefix, got %q", buf.String())
	}
}

func TestLoggerFormattedMethods(t *testing.T) {
	var buf bytes.Buffer
	l := &logger{
		logger:   log.New(&buf, "", 0),
		logLevel: DEBUG,
	}

	l.debugf("value=%d", 42)
	if !strings.Contains(buf.String(), "value=42") {
		t.Errorf("debugf did not format correctly, got %q", buf.String())
	}

	buf.Reset()
	l.infof("name=%s", "test")
	if !strings.Contains(buf.String(), "name=test") {
		t.Errorf("infof did not format correctly, got %q", buf.String())
	}

	buf.Reset()
	l.warnf("count=%d", 0)
	if !strings.Contains(buf.String(), "count=0") {
		t.Errorf("warnf did not format correctly, got %q", buf.String())
	}

	buf.Reset()
	l.errorf("err=%s", "failure")
	if !strings.Contains(buf.String(), "err=failure") {
		t.Errorf("errorf did not format correctly, got %q", buf.String())
	}
}
