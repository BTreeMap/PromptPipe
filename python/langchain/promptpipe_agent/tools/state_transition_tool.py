"""State transition tool for conversation flow."""

from typing import Any

from langchain.tools import BaseTool
from pydantic import BaseModel, Field

from promptpipe_agent.models.schemas import ConversationState
from promptpipe_agent.models.state_manager import StateManager


class StateTransitionInput(BaseModel):
    """Input for state transition tool."""

    participant_id: str = Field(description="The participant ID")
    new_state: str = Field(
        description="The new conversation state (COORDINATOR, INTAKE, or FEEDBACK)"
    )


class StateTransitionTool(BaseTool):
    """Tool for managing conversation state transitions."""

    name: str = "transition_state"
    description: str = (
        "Transition the conversation to a different state. Use this when you need to "
        "hand off the conversation to a different module. States: COORDINATOR (general chat), "
        "INTAKE (collecting user profile), FEEDBACK (tracking habit feedback)."
    )
    args_schema: type[BaseModel] = StateTransitionInput
    state_manager: StateManager

    def _run(self, participant_id: str, new_state: str) -> str:
        """Execute the state transition."""
        try:
            state = ConversationState(new_state)
            self.state_manager.set_current_state(participant_id, state)
            return f"Successfully transitioned to {new_state} state."
        except ValueError:
            return f"Error: Invalid state '{new_state}'. Valid states are: COORDINATOR, INTAKE, FEEDBACK"
        except Exception as e:
            return f"Error transitioning state: {str(e)}"

    async def _arun(self, participant_id: str, new_state: str) -> str:
        """Async version (not implemented, falls back to sync)."""
        return self._run(participant_id, new_state)
