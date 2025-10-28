"""Unit tests for tools."""

import os
import tempfile

import pytest

from promptpipe_agent.models.schemas import ConversationState, UserProfile
from promptpipe_agent.models.state_manager import SQLiteStateManager
from promptpipe_agent.tools.profile_save_tool import ProfileSaveTool
from promptpipe_agent.tools.state_transition_tool import StateTransitionTool


@pytest.fixture
def temp_db():
    """Create a temporary database for testing."""
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        db_path = f.name
    yield db_path
    # Cleanup
    if os.path.exists(db_path):
        os.unlink(db_path)


@pytest.fixture
def state_manager(temp_db):
    """Create a state manager with temporary database."""
    manager = SQLiteStateManager(temp_db)
    # Initialize tables
    with manager._get_connection() as conn:
        cursor = conn.cursor()
        cursor.executescript("""
            CREATE TABLE IF NOT EXISTS flow_states (
                participant_id TEXT NOT NULL,
                flow_type TEXT NOT NULL,
                state TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                PRIMARY KEY (participant_id, flow_type)
            );
            CREATE TABLE IF NOT EXISTS user_profiles (
                participant_id TEXT PRIMARY KEY,
                profile_data TEXT,
                updated_at TEXT
            );
        """)
        conn.commit()
    return manager


def test_state_transition_tool(state_manager):
    """Test StateTransitionTool."""
    tool = StateTransitionTool(state_manager=state_manager)
    participant_id = "test_123"

    # Test transition to INTAKE
    result = tool._run(participant_id, "INTAKE")
    assert "Successfully transitioned" in result
    
    # Verify state changed
    state = state_manager.get_current_state(participant_id)
    assert state == ConversationState.INTAKE


def test_state_transition_tool_invalid_state(state_manager):
    """Test StateTransitionTool with invalid state."""
    tool = StateTransitionTool(state_manager=state_manager)
    participant_id = "test_123"

    # Test invalid state
    result = tool._run(participant_id, "INVALID_STATE")
    assert "Error: Invalid state" in result


def test_profile_save_tool_new_profile(state_manager):
    """Test ProfileSaveTool creating new profile."""
    tool = ProfileSaveTool(state_manager=state_manager)
    participant_id = "test_123"

    # Save profile
    result = tool._run(
        participant_id,
        habit_domain="fitness",
        prompt_anchor="morning coffee",
    )
    assert "Profile saved successfully" in result
    assert "habit_domain" in result

    # Verify profile saved
    profile = state_manager.get_user_profile(participant_id)
    assert profile is not None
    assert profile.habit_domain == "fitness"
    assert profile.prompt_anchor == "morning coffee"


def test_profile_save_tool_update_profile(state_manager):
    """Test ProfileSaveTool updating existing profile."""
    tool = ProfileSaveTool(state_manager=state_manager)
    participant_id = "test_123"

    # Create initial profile
    initial_profile = UserProfile(
        participant_id=participant_id,
        habit_domain="fitness",
    )
    state_manager.save_user_profile(initial_profile)

    # Update with new field
    result = tool._run(participant_id, prompt_anchor="afternoon walk")
    assert "Profile saved successfully" in result

    # Verify update
    profile = state_manager.get_user_profile(participant_id)
    assert profile.habit_domain == "fitness"  # Preserved
    assert profile.prompt_anchor == "afternoon walk"  # Added


def test_profile_save_tool_partial_update(state_manager):
    """Test ProfileSaveTool with partial updates."""
    tool = ProfileSaveTool(state_manager=state_manager)
    participant_id = "test_123"

    # Save only some fields
    result = tool._run(
        participant_id,
        habit_domain="mindfulness",
        preferred_time="8am",
    )
    assert "Profile saved successfully" in result

    # Verify only specified fields are set
    profile = state_manager.get_user_profile(participant_id)
    assert profile.habit_domain == "mindfulness"
    assert profile.preferred_time == "8am"
    assert profile.prompt_anchor is None  # Not set
