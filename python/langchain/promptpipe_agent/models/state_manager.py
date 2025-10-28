"""State management for conversation flow."""

import json
import os
import sqlite3
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Any, Optional

from promptpipe_agent.config import settings
from promptpipe_agent.models.schemas import (
    ConversationHistory,
    ConversationMessage,
    ConversationState,
    UserProfile,
)


class StateManager(ABC):
    """Abstract interface for managing conversation state."""

    @abstractmethod
    def get_current_state(self, participant_id: str) -> Optional[ConversationState]:
        """Get the current conversation state for a participant."""
        pass

    @abstractmethod
    def set_current_state(self, participant_id: str, state: ConversationState) -> None:
        """Set the current conversation state for a participant."""
        pass

    @abstractmethod
    def get_conversation_history(self, participant_id: str) -> ConversationHistory:
        """Get the conversation history for a participant."""
        pass

    @abstractmethod
    def add_message(
        self, participant_id: str, role: str, content: str
    ) -> None:
        """Add a message to the conversation history."""
        pass

    @abstractmethod
    def get_user_profile(self, participant_id: str) -> Optional[UserProfile]:
        """Get the user profile for a participant."""
        pass

    @abstractmethod
    def save_user_profile(self, profile: UserProfile) -> None:
        """Save or update a user profile."""
        pass

    @abstractmethod
    def get_state_data(self, participant_id: str, key: str) -> Optional[Any]:
        """Get arbitrary state data by key."""
        pass

    @abstractmethod
    def set_state_data(self, participant_id: str, key: str, value: Any) -> None:
        """Set arbitrary state data by key."""
        pass


class SQLiteStateManager(StateManager):
    """SQLite-based implementation of StateManager.
    
    This connects to the Go application's SQLite database to share state.
    """

    def __init__(self, db_path: Optional[str] = None):
        """Initialize the SQLite state manager.
        
        Args:
            db_path: Path to the SQLite database. If None, uses the default from settings.
        """
        if db_path is None:
            db_path = os.path.join(settings.promptpipe_state_dir, "state.db")
        
        self.db_path = db_path
        # Ensure the database directory exists
        Path(self.db_path).parent.mkdir(parents=True, exist_ok=True)
        self._init_db()

    def _init_db(self) -> None:
        """Initialize database tables if they don't exist."""
        # The tables should already exist from the Go application
        # This is just a safety check
        pass

    def _get_connection(self) -> sqlite3.Connection:
        """Get a database connection."""
        conn = sqlite3.connect(self.db_path)
        conn.row_factory = sqlite3.Row
        return conn

    def get_current_state(self, participant_id: str) -> Optional[ConversationState]:
        """Get the current conversation state for a participant."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                "SELECT state FROM flow_states WHERE participant_id = ? AND flow_type = 'conversation'",
                (participant_id,),
            )
            row = cursor.fetchone()
            if row:
                return ConversationState(row[0])
            return None

    def set_current_state(self, participant_id: str, state: ConversationState) -> None:
        """Set the current conversation state for a participant."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                INSERT INTO flow_states (participant_id, flow_type, state, updated_at)
                VALUES (?, 'conversation', ?, datetime('now'))
                ON CONFLICT(participant_id, flow_type) DO UPDATE SET
                    state = excluded.state,
                    updated_at = excluded.updated_at
                """,
                (participant_id, state.value),
            )
            conn.commit()

    def get_conversation_history(self, participant_id: str) -> ConversationHistory:
        """Get the conversation history for a participant."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                SELECT role, content, timestamp FROM conversation_history
                WHERE participant_id = ?
                ORDER BY timestamp ASC
                """,
                (participant_id,),
            )
            messages = []
            for row in cursor.fetchall():
                messages.append(
                    ConversationMessage(
                        role=row[0], content=row[1], timestamp=row[2]
                    )
                )
            return ConversationHistory(messages=messages)

    def add_message(
        self, participant_id: str, role: str, content: str
    ) -> None:
        """Add a message to the conversation history."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                INSERT INTO conversation_history (participant_id, role, content, timestamp)
                VALUES (?, ?, ?, datetime('now'))
                """,
                (participant_id, role, content),
            )
            conn.commit()

    def get_user_profile(self, participant_id: str) -> Optional[UserProfile]:
        """Get the user profile for a participant."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                "SELECT profile_data FROM user_profiles WHERE participant_id = ?",
                (participant_id,),
            )
            row = cursor.fetchone()
            if row and row[0]:
                data = json.loads(row[0])
                return UserProfile(participant_id=participant_id, **data)
            return None

    def save_user_profile(self, profile: UserProfile) -> None:
        """Save or update a user profile."""
        profile_data = profile.model_dump(exclude={"participant_id", "created_at", "updated_at"})
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                INSERT INTO user_profiles (participant_id, profile_data, updated_at)
                VALUES (?, ?, datetime('now'))
                ON CONFLICT(participant_id) DO UPDATE SET
                    profile_data = excluded.profile_data,
                    updated_at = excluded.updated_at
                """,
                (profile.participant_id, json.dumps(profile_data)),
            )
            conn.commit()

    def get_state_data(self, participant_id: str, key: str) -> Optional[Any]:
        """Get arbitrary state data by key."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                SELECT value FROM flow_state_data
                WHERE participant_id = ? AND flow_type = 'conversation' AND key = ?
                """,
                (participant_id, key),
            )
            row = cursor.fetchone()
            if row and row[0]:
                return json.loads(row[0])
            return None

    def set_state_data(self, participant_id: str, key: str, value: Any) -> None:
        """Set arbitrary state data by key."""
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                INSERT INTO flow_state_data (participant_id, flow_type, key, value, updated_at)
                VALUES (?, 'conversation', ?, ?, datetime('now'))
                ON CONFLICT(participant_id, flow_type, key) DO UPDATE SET
                    value = excluded.value,
                    updated_at = excluded.updated_at
                """,
                (participant_id, key, json.dumps(value)),
            )
            conn.commit()
