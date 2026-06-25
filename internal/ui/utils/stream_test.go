package utils

import "testing"

func TestWaitForLogLine(t *testing.T) {
	ch := make(chan string, 1)
	ch <- "hello"

	msg := WaitForLog(ch)()
	lm, ok := msg.(LogMsg)
	if !ok {
		t.Fatalf("got %T, want LogMsg", msg)
	}
	if lm.Line != "hello" {
		t.Errorf("Line = %q, want hello", lm.Line)
	}
	if lm.Ch != ch {
		t.Error("LogMsg.Ch does not carry the source channel")
	}
}

func TestWaitForLogClosed(t *testing.T) {
	ch := make(chan string)
	close(ch)

	msg := WaitForLog(ch)()
	cm, ok := msg.(LogClosedMsg)
	if !ok {
		t.Fatalf("got %T, want LogClosedMsg", msg)
	}
	if cm.Ch != ch {
		t.Error("LogClosedMsg.Ch does not carry the source channel")
	}
}
