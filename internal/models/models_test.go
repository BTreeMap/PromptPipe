package models

import "testing"

func TestPromptJSONTags(t *testing.T) {
	p := Prompt{To: "+123", Cron: "* * * * *", Body: "hi"}
	if p.To != "+123" || p.Cron != "* * * * *" || p.Body != "hi" {
		t.Error("Prompt struct fields not set correctly")
	}
}

func TestReceiptJSONTags(t *testing.T) {
	r := Receipt{To: "+123", Status: "sent", Time: 123456}
	if r.To != "+123" || r.Status != "sent" || r.Time != 123456 {
		t.Error("Receipt struct fields not set correctly")
	}
}
