package models

import (
	"encoding/json"
	"testing"
)

func TestPromptJSONTags(t *testing.T) {
	p := Prompt{
		To:            "+123",
		Cron:          "* * * * *",
		Type:          PromptTypeStatic,
		State:         "initial",
		Body:          "hi",
		SystemPrompt:  "system",
		UserPrompt:    "user",
		BranchOptions: []BranchOption{{Label: "Option 1", Body: "Branch Body 1"}},
	}
	expectedJSON := `{"to":"+123","cron":"* * * * *","type":"static","state":"initial","body":"hi","system_prompt":"system","user_prompt":"user","branch_options":[{"label":"Option 1","body":"Branch Body 1"}]}`

	jsonData, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Error marshaling Prompt to JSON: %v", err)
	}

	if string(jsonData) != expectedJSON {
		t.Errorf("JSON marshaling of Prompt was incorrect.\nGot: %s\nWant: %s", string(jsonData), expectedJSON)
	}

	var pUnmarshaled Prompt
	err = json.Unmarshal([]byte(expectedJSON), &pUnmarshaled)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON to Prompt: %v", err)
	}

	if pUnmarshaled.To != p.To || pUnmarshaled.Cron != p.Cron || pUnmarshaled.Type != p.Type || pUnmarshaled.State != p.State || pUnmarshaled.Body != p.Body || pUnmarshaled.SystemPrompt != p.SystemPrompt || pUnmarshaled.UserPrompt != p.UserPrompt || len(pUnmarshaled.BranchOptions) != len(p.BranchOptions) || pUnmarshaled.BranchOptions[0].Label != p.BranchOptions[0].Label || pUnmarshaled.BranchOptions[0].Body != p.BranchOptions[0].Body {
		t.Errorf("JSON unmarshaling of Prompt was incorrect.\nGot: %+v\nWant: %+v", pUnmarshaled, p)
	}
}

func TestReceiptJSONTags(t *testing.T) {
	r := Receipt{To: "+123", Status: StatusTypeSent, Time: 123456}
	expectedJSON := `{"to":"+123","status":"sent","time":123456}`

	jsonData, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Error marshaling Receipt to JSON: %v", err)
	}

	if string(jsonData) != expectedJSON {
		t.Errorf("JSON marshaling of Receipt was incorrect.\nGot: %s\nWant: %s", string(jsonData), expectedJSON)
	}

	var rUnmarshaled Receipt
	err = json.Unmarshal([]byte(expectedJSON), &rUnmarshaled)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON to Receipt: %v", err)
	}

	if rUnmarshaled.To != r.To || rUnmarshaled.Status != r.Status || rUnmarshaled.Time != r.Time {
		t.Errorf("JSON unmarshaling of Receipt was incorrect.\nGot: %+v\nWant: %+v", rUnmarshaled, r)
	}
}

func TestResponseJSONTags(t *testing.T) {
	resp := Response{From: "+123", Body: "hello", Time: 123456}
	expectedJSON := `{"from":"+123","body":"hello","time":123456}`

	jsonData, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Error marshaling Response to JSON: %v", err)
	}

	if string(jsonData) != expectedJSON {
		t.Errorf("JSON marshaling of Response was incorrect.\nGot: %s\nWant: %s", string(jsonData), expectedJSON)
	}

	var respUnmarshaled Response
	err = json.Unmarshal([]byte(expectedJSON), &respUnmarshaled)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON to Response: %v", err)
	}

	if respUnmarshaled.From != resp.From || respUnmarshaled.Body != resp.Body || respUnmarshaled.Time != resp.Time {
		t.Errorf("JSON unmarshaling of Response was incorrect.\nGot: %+v\nWant: %+v", respUnmarshaled, resp)
	}
}
