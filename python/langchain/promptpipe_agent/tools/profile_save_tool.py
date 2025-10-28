"""Profile save tool for storing user profile data."""

from datetime import datetime
from typing import Optional

from langchain.tools import BaseTool
from pydantic import BaseModel, Field

from promptpipe_agent.models.schemas import UserProfile
from promptpipe_agent.models.state_manager import StateManager


class ProfileSaveInput(BaseModel):
    """Input for profile save tool."""

    participant_id: str = Field(description="The participant ID")
    habit_domain: Optional[str] = Field(
        default=None, description="The habit domain (e.g., 'fitness', 'mindfulness')"
    )
    prompt_anchor: Optional[str] = Field(
        default=None,
        description="The prompt anchor - a natural moment to do the habit (e.g., 'waiting for coffee')",
    )
    motivational_frame: Optional[str] = Field(
        default=None,
        description="Why this matters to the user (e.g., 'feel more in control')",
    )
    preferred_time: Optional[str] = Field(
        default=None, description="Preferred time for prompts (e.g., '8-9am', '11:15am')"
    )
    other_personalization: Optional[str] = Field(
        default=None, description="Any other personalization information"
    )


class ProfileSaveTool(BaseTool):
    """Tool for saving and retrieving user profiles."""

    name: str = "save_user_profile"
    description: str = (
        "Save or update the user's profile with their personalization preferences. "
        "Use this after collecting any profile information from the user. "
        "You can update individual fields or multiple fields at once."
    )
    args_schema: type[BaseModel] = ProfileSaveInput
    state_manager: StateManager

    def _run(
        self,
        participant_id: str,
        habit_domain: Optional[str] = None,
        prompt_anchor: Optional[str] = None,
        motivational_frame: Optional[str] = None,
        preferred_time: Optional[str] = None,
        other_personalization: Optional[str] = None,
    ) -> str:
        """Execute the profile save."""
        try:
            # Get existing profile or create new one
            existing_profile = self.state_manager.get_user_profile(participant_id)

            if existing_profile:
                # Update only provided fields
                profile_dict = existing_profile.model_dump()
                if habit_domain is not None:
                    profile_dict["habit_domain"] = habit_domain
                if prompt_anchor is not None:
                    profile_dict["prompt_anchor"] = prompt_anchor
                if motivational_frame is not None:
                    profile_dict["motivational_frame"] = motivational_frame
                if preferred_time is not None:
                    profile_dict["preferred_time"] = preferred_time
                if other_personalization is not None:
                    profile_dict["other_personalization"] = other_personalization
                profile_dict["updated_at"] = datetime.now()
                profile = UserProfile(**profile_dict)
            else:
                # Create new profile
                profile = UserProfile(
                    participant_id=participant_id,
                    habit_domain=habit_domain,
                    prompt_anchor=prompt_anchor,
                    motivational_frame=motivational_frame,
                    preferred_time=preferred_time,
                    other_personalization=other_personalization,
                )

            self.state_manager.save_user_profile(profile)

            # Build a summary of what was saved
            saved_fields = []
            if habit_domain:
                saved_fields.append(f"habit_domain: {habit_domain}")
            if prompt_anchor:
                saved_fields.append(f"prompt_anchor: {prompt_anchor}")
            if motivational_frame:
                saved_fields.append(f"motivational_frame: {motivational_frame}")
            if preferred_time:
                saved_fields.append(f"preferred_time: {preferred_time}")
            if other_personalization:
                saved_fields.append(f"other_personalization: {other_personalization}")

            if saved_fields:
                return f"Profile saved successfully. Updated fields: {', '.join(saved_fields)}"
            else:
                return "Profile save called but no fields were updated."

        except Exception as e:
            return f"Error saving profile: {str(e)}"

    async def _arun(
        self,
        participant_id: str,
        habit_domain: Optional[str] = None,
        prompt_anchor: Optional[str] = None,
        motivational_frame: Optional[str] = None,
        preferred_time: Optional[str] = None,
        other_personalization: Optional[str] = None,
    ) -> str:
        """Async version (not implemented, falls back to sync)."""
        return self._run(
            participant_id,
            habit_domain,
            prompt_anchor,
            motivational_frame,
            preferred_time,
            other_personalization,
        )
