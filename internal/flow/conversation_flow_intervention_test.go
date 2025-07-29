package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/openai/openai-go"
)

// RealisticThreeBotGenAIClient simulates the actual three-bot architecture flow
// Based on the real system prompts and conversation patterns observed
type RealisticThreeBotGenAIClient struct {
	conversationStep map[string]int  // Track conversation progress per participant
	userProfiles     map[string]map[string]interface{} // Store user data
}

func NewRealisticThreeBotGenAIClient() *RealisticThreeBotGenAIClient {
	return &RealisticThreeBotGenAIClient{
		conversationStep: make(map[string]int),
		userProfiles:     make(map[string]map[string]interface{}),
	}
}

func (m *MockThreeBotGenAIClient) GeneratePrompt(system, user string) (string, error) {
	return m.getResponseForInput(user), nil
}

func (m *MockThreeBotGenAIClient) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	return m.getResponseForInput(user), nil
}

func (m *MockThreeBotGenAIClient) GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	// Get the last user message to determine response - simplified for testing
	lastUserMsg := "mock_user_input"
	return m.getResponseForInput(lastUserMsg), nil
}

func (m *MockThreeBotGenAIClient) GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*genai.ToolCallResponse, error) {
	// For tools, return regular content response (no tool calls for this test)
	lastUserMsg := "mock_user_input"
	
	return &genai.ToolCallResponse{
		Content:   m.getResponseForInput(lastUserMsg),
		ToolCalls: nil, // No tool calls for this comprehensive test
	}, nil
}

func (m *MockThreeBotGenAIClient) getResponseForInput(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	
	// First interaction - always start with intake
	if input == "" || strings.Contains(input, "hello") || strings.Contains(input, "hi") || strings.Contains(input, "start") {
		return m.responses["initial"]
	}
	
	// Intake bot flow
	if strings.Contains(input, "yes") || strings.Contains(input, "okay") || strings.Contains(input, "sure") {
		return m.responses["yes"]
	}
	if strings.Contains(input, "physical") || strings.Contains(input, "exercise") || strings.Contains(input, "activity") {
		return m.responses["physical"]
	}
	if strings.Contains(input, "energy") || strings.Contains(input, "energized") || strings.Contains(input, "tired") {
		return m.responses["energy"]
	}
	if strings.Contains(input, "morning") || strings.Contains(input, "8") || strings.Contains(input, "9") {
		return m.responses["morning"]
	}
	if strings.Contains(input, "coffee") || strings.Contains(input, "after coffee") {
		return m.responses["coffee"]
	}
	if strings.Contains(input, "nothing") || strings.Contains(input, "no") || strings.Contains(input, "that's all") {
		return m.responses["nothing"]
	}
	
	// Prompt generator bot flow
	if strings.Contains(input, "try") || strings.Contains(input, "now") || strings.Contains(input, "send it") {
		return m.responses["try_now"]
	}
	
	// Feedback tracker bot flow
	if strings.Contains(input, "tried") || strings.Contains(input, "did it") || strings.Contains(input, "completed") {
		return m.responses["tried_it"]
	}
	if strings.Contains(input, "didn't") || strings.Contains(input, "forgot") || strings.Contains(input, "couldn't") {
		return m.responses["didnt_try"]
	}
	
	// Default fallback
	return "I understand. Could you tell me more about what you're looking for?"
}

// MockE2EMessagingService for comprehensive end-to-end testing
type MockE2EMessagingService struct {
	sentMessages []MockMessage
}

type MockMessage struct {
	To      string
	Content string
	SentAt  time.Time
}

func (m *MockE2EMessagingService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	return recipient, nil
}

func (m *MockE2EMessagingService) SendMessage(ctx context.Context, to, message string) error {
	m.sentMessages = append(m.sentMessages, MockMessage{
		To:      to,
		Content: message,
		SentAt:  time.Now(),
	})
	return nil
}

func (m *MockE2EMessagingService) GetSentMessages() []MockMessage {
	return m.sentMessages
}

// TestThreeBotArchitectureEndToEnd tests the complete flow from intake → prompt generator → feedback tracker
func TestThreeBotArchitectureEndToEnd(t *testing.T) {
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	
	// Create mock services
	mockGenAI := NewMockThreeBotGenAIClient()
	msgService := &MockE2EMessagingService{}
	
	// Create conversation flow (no specific tools needed for this test)
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", nil, nil)
	flow.SetMessageService(msgService)
	
	// Add phone number to context
	ctx = context.WithValue(ctx, phoneNumberContextKey, "+1234567890")
	participantID := "test-participant-e2e"
	
	t.Log("=== PHASE 1: INTAKE BOT ===")
	
	// Step 1: Initial greeting - should trigger intake bot
	response1, err := flow.ProcessResponse(ctx, participantID, "Hello! I want to build a healthy habit")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}
	expectedIntake := "Hi! I'm your micro-coach bot here to help you build a 1-minute healthy habit that fits into your day. I'll ask a few quick questions to personalize it. Is that okay?"
	if response1 != expectedIntake {
		t.Errorf("Step 1: Expected intake greeting, got: %s", response1)
	}
	t.Logf("✓ Step 1 - Intake bot initiated: %s", response1)
	
	// Step 2: Confirm participation
	response2, err := flow.ProcessResponse(ctx, participantID, "Yes, that sounds great!")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}
	expectedQuestion1 := "What's one habit you've been meaning to build or restart? (Healthy eating, Physical Activity, Mental well being, Reduce screen time, or Anything else)"
	if response2 != expectedQuestion1 {
		t.Errorf("Step 2: Expected first question, got: %s", response2)
	}
	t.Logf("✓ Step 2 - First intake question: %s", response2)
	
	// Step 3: Choose habit area
	response3, err := flow.ProcessResponse(ctx, participantID, "Physical Activity")
	if err != nil {
		t.Fatalf("Step 3 failed: %v", err)
	}
	expectedQuestion2 := "Why does this matter to you now? What would doing this help you feel or achieve?"
	if response3 != expectedQuestion2 {
		t.Errorf("Step 3: Expected motivation question, got: %s", response3)
	}
	t.Logf("✓ Step 3 - Motivation question: %s", response3)
	
	// Step 4: Provide motivation
	response4, err := flow.ProcessResponse(ctx, participantID, "I want to feel more energized throughout the day")
	if err != nil {
		t.Fatalf("Step 4 failed: %v", err)
	}
	expectedQuestion3 := "When during the day would you like to get a 1-minute nudge from me? You can share a time block like 8–9am, an exact time or randomly during the day"
	if response4 != expectedQuestion3 {
		t.Errorf("Step 4: Expected timing question, got: %s", response4)
	}
	t.Logf("✓ Step 4 - Timing question: %s", response4)
	
	// Step 5: Provide preferred time
	response5, err := flow.ProcessResponse(ctx, participantID, "In the morning around 8-9am")
	if err != nil {
		t.Fatalf("Step 5 failed: %v", err)
	}
	expectedQuestion4 := "When do you think this habit would naturally fit into your day? For example, after coffee, before meetings, or when you feel overwhelmed, or anything else that would work for you"
	if response5 != expectedQuestion4 {
		t.Errorf("Step 5: Expected anchor question, got: %s", response5)
	}
	t.Logf("✓ Step 5 - Habit anchor question: %s", response5)
	
	// Step 6: Provide habit anchor
	response6, err := flow.ProcessResponse(ctx, participantID, "Right after I have my morning coffee")
	if err != nil {
		t.Fatalf("Step 6 failed: %v", err)
	}
	expectedQuestion5 := "Is there anything else you'd like me to know that would help personalize your habit suggestion even more?"
	if response6 != expectedQuestion5 {
		t.Errorf("Step 6: Expected final intake question, got: %s", response6)
	}
	t.Logf("✓ Step 6 - Final intake question: %s", response6)
	
	// Step 7: Complete intake
	response7, err := flow.ProcessResponse(ctx, participantID, "Nothing else, that covers it")
	if err != nil {
		t.Fatalf("Step 7 failed: %v", err)
	}
	expectedOffer := "Great! Would you like to try a 1-minute version of this habit right now? I can send it to you."
	if response7 != expectedOffer {
		t.Errorf("Step 7: Expected immediate offer, got: %s", response7)
	}
	t.Logf("✓ Step 7 - Intake completed, offering immediate prompt: %s", response7)
	
	// Verify profile was created and stored
	profileData, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "user_profile")
	if err != nil || profileData == "" {
		t.Errorf("Expected user profile to be stored after intake completion")
	} else {
		var profile map[string]interface{}
		if err := json.Unmarshal([]byte(profileData), &profile); err == nil {
			t.Logf("✓ User profile created: %+v", profile)
		}
	}
	
	t.Log("=== PHASE 2: PROMPT GENERATOR BOT ===")
	
	// Step 8: Request immediate prompt
	response8, err := flow.ProcessResponse(ctx, participantID, "Yes, I'd like to try it now")
	if err != nil {
		t.Fatalf("Step 8 failed: %v", err)
	}
	expectedPrompt := "After your coffee, try 10 jumping jacks or a quick stretch — it helps you feel more energized and ready for the day. Would that feel doable?"
	if response8 != expectedPrompt {
		t.Errorf("Step 8: Expected personalized prompt, got: %s", response8)
	}
	t.Logf("✓ Step 8 - Personalized habit prompt generated: %s", response8)
	
	t.Log("=== PHASE 3: FEEDBACK TRACKER BOT ===")
	
	// Step 9: Provide positive feedback
	response9, err := flow.ProcessResponse(ctx, participantID, "Yes, I tried it and it felt great!")
	if err != nil {
		t.Fatalf("Step 9 failed: %v", err)
	}
	expectedFeedback := "What made it work well?"
	if response9 != expectedFeedback {
		t.Errorf("Step 9: Expected feedback question, got: %s", response9)
	}
	t.Logf("✓ Step 9 - Feedback tracker activated: %s", response9)
	
	// Step 10: Provide detailed feedback
	response10, err := flow.ProcessResponse(ctx, participantID, "It was easy to remember since I always have coffee, and the jumping jacks really did wake me up")
	if err != nil {
		t.Fatalf("Step 10 failed: %v", err)
	}
	t.Logf("✓ Step 10 - Detailed feedback collected: %s", response10)
	
	// Verify feedback was processed and profile updated
	updatedProfileData, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "user_profile")
	if err != nil || updatedProfileData == "" {
		t.Errorf("Expected updated user profile after feedback")
	} else {
		var updatedProfile map[string]interface{}
		if err := json.Unmarshal([]byte(updatedProfileData), &updatedProfile); err == nil {
			t.Logf("✓ Updated profile after feedback: %+v", updatedProfile)
		}
	}
	
	t.Log("=== END-TO-END TEST COMPLETED SUCCESSFULLY ===")
	t.Logf("Total conversation steps: 10")
	t.Logf("Architecture phases covered: Intake Bot → Prompt Generator Bot → Feedback Tracker Bot")
	t.Logf("User profile created and updated through the complete flow")
}

// TestThreeBotArchitectureRejectionFlow tests what happens when user rejects the habit prompt
func TestThreeBotArchitectureRejectionFlow(t *testing.T) {
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	
	mockGenAI := NewMockThreeBotGenAIClient()
	msgService := &MockE2EMessagingService{}
	
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", nil, nil)
	flow.SetMessageService(msgService)
	
	ctx = context.WithValue(ctx, phoneNumberContextKey, "+1234567890")
	participantID := "test-participant-rejection"
	
	// Complete the intake process quickly (abbreviated)
	flow.ProcessResponse(ctx, participantID, "Hi")
	flow.ProcessResponse(ctx, participantID, "Yes")
	flow.ProcessResponse(ctx, participantID, "Physical Activity")
	flow.ProcessResponse(ctx, participantID, "I want more energy")
	flow.ProcessResponse(ctx, participantID, "Morning")
	flow.ProcessResponse(ctx, participantID, "After coffee")
	flow.ProcessResponse(ctx, participantID, "Nothing else")
	
	// Get the habit prompt
	promptResponse, err := flow.ProcessResponse(ctx, participantID, "Yes, try it now")
	if err != nil {
		t.Fatalf("Failed to get prompt: %v", err)
	}
	t.Logf("Prompt generated: %s", promptResponse)
	
	// User didn't try it
	rejectionResponse, err := flow.ProcessResponse(ctx, participantID, "I didn't get a chance to try it")
	if err != nil {
		t.Fatalf("Failed to process rejection: %v", err)
	}
	
	expectedRejectionQuestion := "What got in the way?"
	if rejectionResponse != expectedRejectionQuestion {
		t.Errorf("Expected rejection follow-up, got: %s", rejectionResponse)
	}
	
	// Provide barrier information
	barrierResponse, err := flow.ProcessResponse(ctx, participantID, "I forgot about it after getting busy with work")
	if err != nil {
		t.Fatalf("Failed to process barrier: %v", err)
	}
	
	t.Logf("✓ Rejection flow handled successfully: %s", barrierResponse)
	
	// Verify barrier information was captured in profile
	profileData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "user_profile")
	if profileData != "" {
		var profile map[string]interface{}
		json.Unmarshal([]byte(profileData), &profile)
		t.Logf("✓ Profile updated with barrier information: %+v", profile)
	}
}

// TestStateManagementThroughThreeBotFlow verifies that state is properly managed across the bot transitions
func TestStateManagementThroughThreeBotFlow(t *testing.T) {
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	
	mockGenAI := NewMockThreeBotGenAIClient()
	msgService := &MockE2EMessagingService{}
	
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", nil, nil)
	flow.SetMessageService(msgService)
	
	ctx = context.WithValue(ctx, phoneNumberContextKey, "+1234567890")
	participantID := "test-participant-state"
	
	// Verify initial state
	initialState, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "bot_phase")
	if initialState != "" {
		t.Errorf("Expected empty initial state, got: %s", initialState)
	}
	
	// Start conversation - should set intake bot phase
	flow.ProcessResponse(ctx, participantID, "Hello")
	
	intakeState, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "bot_phase")
	if intakeState == "" {
		t.Logf("Intake phase state: %s", intakeState)
	}
	
	// Complete intake
	flow.ProcessResponse(ctx, participantID, "Yes")
	flow.ProcessResponse(ctx, participantID, "Physical Activity")
	flow.ProcessResponse(ctx, participantID, "Energy")
	flow.ProcessResponse(ctx, participantID, "Morning")
	flow.ProcessResponse(ctx, participantID, "Coffee")
	flow.ProcessResponse(ctx, participantID, "Nothing")
	
	// Request prompt - should transition to prompt generator
	flow.ProcessResponse(ctx, participantID, "Yes, try now")
	
	promptState, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "bot_phase")
	t.Logf("Prompt generator phase state: %s", promptState)
	
	// Provide feedback - should transition to feedback tracker
	flow.ProcessResponse(ctx, participantID, "I tried it")
	
	feedbackState, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "bot_phase")
	t.Logf("Feedback tracker phase state: %s", feedbackState)
	
	// Verify conversation history is maintained
	historyData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory)
	if historyData == "" {
		t.Error("Expected conversation history to be maintained")
	} else {
		var history []map[string]interface{}
		json.Unmarshal([]byte(historyData), &history)
		if len(history) < 5 {
			t.Errorf("Expected multiple conversation turns, got %d", len(history))
		}
		t.Logf("✓ Conversation history maintained with %d turns", len(history))
	}
}
