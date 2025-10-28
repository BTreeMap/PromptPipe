"""Unit tests for data models."""

from datetime import datetime

import pytest

from promptpipe_agent.models.schemas import (
    ConversationHistory,
    ConversationMessage,
    ConversationState,
    MessageRole,
    UserProfile,
)


def test_conversation_message():
    """Test ConversationMessage model."""
    msg = ConversationMessage(
        role=MessageRole.USER,
        content="Hello, world!",
    )
    assert msg.role == MessageRole.USER
    assert msg.content == "Hello, world!"
    assert isinstance(msg.timestamp, datetime)


def test_conversation_history():
    """Test ConversationHistory model."""
    history = ConversationHistory(
        messages=[
            ConversationMessage(role=MessageRole.USER, content="Hi"),
            ConversationMessage(role=MessageRole.ASSISTANT, content="Hello!"),
        ]
    )
    assert len(history.messages) == 2
    assert history.messages[0].role == MessageRole.USER
    assert history.messages[1].role == MessageRole.ASSISTANT


def test_user_profile():
    """Test UserProfile model."""
    profile = UserProfile(
        participant_id="test_123",
        habit_domain="fitness",
        prompt_anchor="waiting for coffee",
        motivational_frame="feel more energized",
        preferred_time="9am",
    )
    assert profile.participant_id == "test_123"
    assert profile.habit_domain == "fitness"


def test_user_profile_to_context_string():
    """Test UserProfile to_context_string method."""
    profile = UserProfile(
        participant_id="test_123",
        habit_domain="fitness",
        prompt_anchor="waiting for coffee",
    )
    context = profile.to_context_string()
    assert "Habit Domain: fitness" in context
    assert "Prompt Anchor: waiting for coffee" in context


def test_user_profile_empty_context():
    """Test UserProfile with no data."""
    profile = UserProfile(participant_id="test_123")
    context = profile.to_context_string()
    assert context == "No profile data available."


def test_conversation_state_enum():
    """Test ConversationState enum."""
    assert ConversationState.COORDINATOR.value == "COORDINATOR"
    assert ConversationState.INTAKE.value == "INTAKE"
    assert ConversationState.FEEDBACK.value == "FEEDBACK"
