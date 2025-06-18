package models

import (
	"encoding/json"
	"testing"
)

func TestPromptJSONTags(t *testing.T) {
	p := Prompt{
		To:            "+1234567890",
		Cron:          "* * * * *",
		Type:          PromptTypeStatic,
		State:         "initial",
		Body:          "hi",
		SystemPrompt:  "system",
		UserPrompt:    "user",
		BranchOptions: []BranchOption{{Label: "Option 1", Body: "Branch Body 1"}},
	}
	expectedJSON := `{"to":"+1234567890","cron":"* * * * *","type":"static","state":"initial","body":"hi","system_prompt":"system","user_prompt":"user","branch_options":[{"label":"Option 1","body":"Branch Body 1"}]}`

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
	r := Receipt{To: "+1234567890", Status: MessageStatusSent, Time: 123456}
	expectedJSON := `{"to":"+1234567890","status":"sent","time":123456}`

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

// Test validation functions
func TestIsValidPromptType(t *testing.T) {
	tests := []struct {
		promptType PromptType
		expected   bool
	}{
		{PromptTypeStatic, true},
		{PromptTypeGenAI, true},
		{PromptTypeBranch, true},
		{PromptTypeCustom, true},
		{PromptType("invalid"), false},
		{PromptType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.promptType), func(t *testing.T) {
			result := IsValidPromptType(tt.promptType)
			if result != tt.expected {
				t.Errorf("IsValidPromptType(%v) = %v; want %v", tt.promptType, result, tt.expected)
			}
		})
	}
}

func TestPromptValidate_Static(t *testing.T) {
	tests := []struct {
		name    string
		prompt  Prompt
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid static prompt",
			prompt:  Prompt{To: "+123", Type: PromptTypeStatic, Body: "Hello world"},
			wantErr: false,
		},
		{
			name:    "static prompt missing recipient",
			prompt:  Prompt{Type: PromptTypeStatic, Body: "Hello world"},
			wantErr: true,
			errMsg:  "recipient cannot be empty",
		},
		{
			name:    "static prompt missing body",
			prompt:  Prompt{To: "+123", Type: PromptTypeStatic},
			wantErr: true,
			errMsg:  "body is required for static prompts",
		},
		{
			name:    "static prompt body too long",
			prompt:  Prompt{To: "+123", Type: PromptTypeStatic, Body: string(make([]byte, MaxPromptBodyLength+1))},
			wantErr: true,
			errMsg:  "prompt body exceeds maximum length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prompt.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v; want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPromptValidate_GenAI(t *testing.T) {
	tests := []struct {
		name    string
		prompt  Prompt
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid genai prompt",
			prompt:  Prompt{To: "+123", Type: PromptTypeGenAI, SystemPrompt: "System", UserPrompt: "User"},
			wantErr: false,
		},
		{
			name:    "genai prompt missing system prompt",
			prompt:  Prompt{To: "+123", Type: PromptTypeGenAI, UserPrompt: "User"},
			wantErr: true,
			errMsg:  "system prompt is required for GenAI prompts",
		},
		{
			name:    "genai prompt missing user prompt",
			prompt:  Prompt{To: "+123", Type: PromptTypeGenAI, SystemPrompt: "System"},
			wantErr: true,
			errMsg:  "user prompt is required for GenAI prompts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prompt.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v; want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPromptValidate_Branch(t *testing.T) {
	validOptions := []BranchOption{
		{Label: "Option A", Body: "Body A"},
		{Label: "Option B", Body: "Body B"},
	}

	tests := []struct {
		name    string
		prompt  Prompt
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid branch prompt",
			prompt:  Prompt{To: "+123", Type: PromptTypeBranch, BranchOptions: validOptions},
			wantErr: false,
		},
		{
			name:    "branch prompt missing options",
			prompt:  Prompt{To: "+123", Type: PromptTypeBranch},
			wantErr: true,
			errMsg:  "branch options are required for branch prompts",
		},
		{
			name:    "branch prompt too few options",
			prompt:  Prompt{To: "+123", Type: PromptTypeBranch, BranchOptions: []BranchOption{{Label: "A", Body: "Body A"}}},
			wantErr: true,
			errMsg:  "insufficient branch options",
		},
		{
			name:    "branch prompt empty label",
			prompt:  Prompt{To: "+123", Type: PromptTypeBranch, BranchOptions: []BranchOption{{Label: "", Body: "Body A"}, {Label: "B", Body: "Body B"}}},
			wantErr: true,
			errMsg:  "branch label cannot be empty",
		},
		{
			name:    "branch prompt empty body",
			prompt:  Prompt{To: "+123", Type: PromptTypeBranch, BranchOptions: []BranchOption{{Label: "A", Body: ""}, {Label: "B", Body: "Body B"}}},
			wantErr: true,
			errMsg:  "branch body cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prompt.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v; want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}
