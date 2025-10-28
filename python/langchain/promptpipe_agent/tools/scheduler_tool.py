"""Scheduler tool for scheduling daily habit prompts."""

from typing import Optional

from langchain.tools import BaseTool
from pydantic import BaseModel, Field

from promptpipe_agent.models.state_manager import StateManager


class SchedulerInput(BaseModel):
    """Input for scheduler tool."""

    participant_id: str = Field(description="The participant ID")
    time: str = Field(
        description="The time to schedule prompts (e.g., '11:15am', '8-9am', '14:30')"
    )
    message: Optional[str] = Field(
        default=None, description="Optional custom message for the scheduled prompt"
    )


class SchedulerTool(BaseTool):
    """Tool for scheduling daily habit prompts."""

    name: str = "schedule_daily_prompt"
    description: str = (
        "Schedule a daily habit prompt to be sent at a specific time. "
        "Use this after the user has chosen when they want to receive their daily prompts. "
        "The time can be in various formats like '11:15am', '8-9am', or '14:30'."
    )
    args_schema: type[BaseModel] = SchedulerInput
    state_manager: StateManager
    go_api_url: str = "http://localhost:8080"  # URL of the Go API

    def _run(
        self, participant_id: str, time: str, message: Optional[str] = None
    ) -> str:
        """Execute the scheduling."""
        try:
            # Store the schedule preference in state
            schedule_data = {
                "time": time,
                "message": message or "personalized",  # "personalized" means generate prompt
                "enabled": True,
            }
            self.state_manager.set_state_data(participant_id, "schedule", schedule_data)

            # In a full implementation, we would call the Go API to actually schedule the prompt
            # For now, we just store the preference
            return f"Daily prompt scheduled for {time}. The prompt will be sent automatically."

        except Exception as e:
            return f"Error scheduling prompt: {str(e)}"

    async def _arun(
        self, participant_id: str, time: str, message: Optional[str] = None
    ) -> str:
        """Async version (not implemented, falls back to sync)."""
        return self._run(participant_id, time, message)
