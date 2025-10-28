"""Integration tests for conversation flow."""

import os
import tempfile

import pytest

from promptpipe_agent.agents.orchestrator import ConversationOrchestrator
from promptpipe_agent.models.schemas import ConversationState
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


@pytest.fixture
def orchestrator(state_manager):
    """Create an orchestrator for testing."""
    # Skip if no OpenAI API key (integration tests need real API)
    if not os.getenv("OPENAI_API_KEY"):
        pytest.skip("OPENAI_API_KEY not set, skipping integration test")
    return ConversationOrchestrator(state_manager)


def test_orchestrator_routing(state_manager):
    """Test that orchestrator routes to correct agent based on state."""
    orchestrator = ConversationOrchestrator(state_manager)
    participant_id = "test_123"

    # Test COORDINATOR state (default)
    response, state = orchestrator.process_message(participant_id, "Hello")
    assert state in [ConversationState.COORDINATOR, ConversationState.CONVERSATION_ACTIVE]
    assert response  # Should have a response

    # Set to INTAKE state
    state_manager.set_current_state(participant_id, ConversationState.INTAKE)
    
    # Process message should go to intake agent
    response, state = orchestrator.process_message(participant_id, "I want to personalize")
    assert state == ConversationState.INTAKE  # Should stay in INTAKE
    assert response  # Should have a response


def test_conversation_history_persistence(state_manager):
    """Test that conversation history is persisted."""
    orchestrator = ConversationOrchestrator(state_manager)
    participant_id = "test_456"

    # Send first message
    response1, _ = orchestrator.process_message(participant_id, "Hello!")

    # Send second message
    response2, _ = orchestrator.process_message(participant_id, "How are you?")

    # Check history
    history = state_manager.get_conversation_history(participant_id)
    assert len(history.messages) >= 4  # At least 2 user messages + 2 assistant messages
    assert history.messages[0].content == "Hello!"
    assert history.messages[2].content == "How are you?"
