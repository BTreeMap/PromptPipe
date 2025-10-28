"""Data models for PromptPipe Agent."""

from datetime import datetime
from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, Field


class ConversationState(str, Enum):
    """Conversation flow states."""

    COORDINATOR = "COORDINATOR"
    INTAKE = "INTAKE"
    FEEDBACK = "FEEDBACK"
    CONVERSATION_ACTIVE = "CONVERSATION_ACTIVE"


class MessageRole(str, Enum):
    """Message roles in conversation."""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"


class ConversationMessage(BaseModel):
    """A single message in the conversation history."""

    role: MessageRole
    content: str
    timestamp: datetime = Field(default_factory=datetime.now)


class ConversationHistory(BaseModel):
    """Full conversation history for a participant."""

    messages: list[ConversationMessage] = Field(default_factory=list)


class UserProfile(BaseModel):
    """User profile containing personalization data."""

    participant_id: str
    habit_domain: Optional[str] = None
    prompt_anchor: Optional[str] = None
    motivational_frame: Optional[str] = None
    preferred_time: Optional[str] = None
    other_personalization: Optional[str] = None
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)

    def to_context_string(self) -> str:
        """Convert profile to a context string for LLM."""
        parts = []
        if self.habit_domain:
            parts.append(f"Habit Domain: {self.habit_domain}")
        if self.prompt_anchor:
            parts.append(f"Prompt Anchor: {self.prompt_anchor}")
        if self.motivational_frame:
            parts.append(f"Motivational Frame: {self.motivational_frame}")
        if self.preferred_time:
            parts.append(f"Preferred Time: {self.preferred_time}")
        if self.other_personalization:
            parts.append(f"Other Personalization: {self.other_personalization}")
        return "\n".join(parts) if parts else "No profile data available."


class ProcessMessageRequest(BaseModel):
    """Request to process a user message."""

    participant_id: str
    message: str
    phone_number: str


class ProcessMessageResponse(BaseModel):
    """Response from processing a user message."""

    response: str
    state: ConversationState
    metadata: dict[str, Any] = Field(default_factory=dict)


class HealthResponse(BaseModel):
    """Health check response."""

    status: str
    version: str = "0.1.0"
