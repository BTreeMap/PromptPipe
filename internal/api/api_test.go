package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendHandler_NotImplemented(t *testing.T) {
	req, _ := http.NewRequest("POST", "/send", bytes.NewBuffer([]byte(`{"to":"+123","body":"hi"}`)))
	rr := httptest.NewRecorder()
	sendHandler(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestScheduleHandler_NotImplemented(t *testing.T) {
	req, _ := http.NewRequest("POST", "/schedule", bytes.NewBuffer([]byte(`{"to":"+123","cron":"* * * * *","body":"hi"}`)))
	rr := httptest.NewRecorder()
	scheduleHandler(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestReceiptsHandler_NotImplemented(t *testing.T) {
	req, _ := http.NewRequest("GET", "/receipts", nil)
	rr := httptest.NewRecorder()
	receiptsHandler(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}
