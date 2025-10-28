"""Unit tests for state manager."""

import os
import tempfile

import pytest

from promptpipe_agent.models.schemas import ConversationState, MessageRole, UserProfile
from promptpipe_agent.models.state_manager import SQLiteStateManager


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
            CREATE TABLE IF NOT EXISTS conversation_history (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                participant_id TEXT NOT NULL,
                role TEXT NOT NULL,
                content TEXT NOT NULL,
                timestamp TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS user_profiles (
                participant_id TEXT PRIMARY KEY,
                profile_data TEXT,
                updated_at TEXT
            );
            CREATE TABLE IF NOT EXISTS flow_state_data (
                participant_id TEXT NOT NULL,
                flow_type TEXT NOT NULL,
                key TEXT NOT NULL,
                value TEXT,
                updated_at TEXT,
                PRIMARY KEY (participant_id, flow_type, key)
            );
        """)
        conn.commit()
    return manager


def test_get_set_current_state(state_manager):
    """Test getting and setting conversation state."""
    participant_id = "test_123"

    # Initially no state
    state = state_manager.get_current_state(participant_id)
    assert state is None

    # Set state
    state_manager.set_current_state(participant_id, ConversationState.INTAKE)

    # Get state
    state = state_manager.get_current_state(participant_id)
    assert state == ConversationState.INTAKE


def test_add_get_messages(state_manager):
    """Test adding and getting conversation messages."""
    participant_id = "test_123"

    # Add messages
    state_manager.add_message(participant_id, MessageRole.USER.value, "Hello")
    state_manager.add_message(participant_id, MessageRole.ASSISTANT.value, "Hi there!")

    # Get history
    history = state_manager.get_conversation_history(participant_id)
    assert len(history.messages) == 2
    assert history.messages[0].content == "Hello"
    assert history.messages[1].content == "Hi there!"


def test_save_get_user_profile(state_manager):
    """Test saving and getting user profile."""
    participant_id = "test_123"

    # Create and save profile
    profile = UserProfile(
        participant_id=participant_id,
        habit_domain="fitness",
        prompt_anchor="waiting for coffee",
    )
    state_manager.save_user_profile(profile)

    # Get profile
    retrieved = state_manager.get_user_profile(participant_id)
    assert retrieved is not None
    assert retrieved.participant_id == participant_id
    assert retrieved.habit_domain == "fitness"
    assert retrieved.prompt_anchor == "waiting for coffee"


def test_update_user_profile(state_manager):
    """Test updating an existing user profile."""
    participant_id = "test_123"

    # Create initial profile
    profile1 = UserProfile(
        participant_id=participant_id,
        habit_domain="fitness",
    )
    state_manager.save_user_profile(profile1)

    # Update profile
    profile2 = UserProfile(
        participant_id=participant_id,
        habit_domain="fitness",
        prompt_anchor="morning coffee",
    )
    state_manager.save_user_profile(profile2)

    # Get updated profile
    retrieved = state_manager.get_user_profile(participant_id)
    assert retrieved.prompt_anchor == "morning coffee"


def test_get_set_state_data(state_manager):
    """Test getting and setting arbitrary state data."""
    participant_id = "test_123"

    # Set data
    state_manager.set_state_data(participant_id, "schedule", {"time": "9am"})

    # Get data
    data = state_manager.get_state_data(participant_id, "schedule")
    assert data == {"time": "9am"}
