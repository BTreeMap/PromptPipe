"""Prompt generator tool for creating personalized habit prompts."""

import os
from typing import Any, Optional

from langchain.tools import BaseTool
from langchain_openai import ChatOpenAI
from pydantic import BaseModel, Field

from promptpipe_agent.config import settings
from promptpipe_agent.models.state_manager import StateManager


class PromptGeneratorInput(BaseModel):
    """Input for prompt generator tool."""

    participant_id: str = Field(description="The participant ID")


class PromptGeneratorTool(BaseTool):
    """Tool for generating personalized habit prompts."""

    name: str = "generate_habit_prompt"
    description: str = (
        "Generate a personalized habit prompt based on the user's profile. "
        "Use this when the user wants to try the habit now or after completing the intake. "
        "The generated prompt will be based on their preferences (anchor, timing, motivational frame)."
    )
    args_schema: type[BaseModel] = PromptGeneratorInput
    state_manager: StateManager
    llm: Optional[ChatOpenAI] = None
    system_prompt: Optional[str] = None

    def model_post_init(self, __context: Any) -> None:
        """Initialize the LLM and load system prompt after model creation."""
        if self.llm is None:
            self.llm = ChatOpenAI(
                model=settings.openai_model,
                temperature=settings.openai_temperature,
                api_key=settings.openai_api_key,
            )
        if self.system_prompt is None:
            self._load_system_prompt()

    def _load_system_prompt(self) -> None:
        """Load the system prompt from file."""
        prompt_file = settings.prompt_generator_prompt_file
        if not os.path.isabs(prompt_file):
            # Make path relative to project root
            base_dir = os.path.dirname(
                os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
            )
            prompt_file = os.path.join(base_dir, prompt_file)

        if os.path.exists(prompt_file):
            with open(prompt_file, "r") as f:
                self.system_prompt = f.read().strip()
        else:
            # Fallback prompt
            self.system_prompt = (
                "Generate a personalized 1-minute habit prompt based on the user's profile. "
                "The prompt should be actionable, specific, and aligned with their preferences."
            )

    def _run(self, participant_id: str) -> str:
        """Execute the prompt generation."""
        try:
            # Get user profile
            profile = self.state_manager.get_user_profile(participant_id)
            if not profile:
                return (
                    "If your phone buzzes, take 50 steps, either walking around or walking in place. "
                    "Active people like you can reach their fitness goals with these tiny steps."
                )

            # Build context from profile
            profile_context = profile.to_context_string()

            # Generate prompt using LLM
            messages = [
                {"role": "system", "content": self.system_prompt},
                {
                    "role": "user",
                    "content": f"User Profile:\n{profile_context}\n\nGenerate a personalized habit prompt.",
                },
            ]

            response = self.llm.invoke(messages)
            generated_prompt = response.content.strip()

            return generated_prompt

        except Exception:
            # Fallback to default prompt on error
            return (
                "If your phone buzzes, take 50 steps, either walking around or walking in place. "
                "Active people like you can reach their fitness goals with these tiny steps."
            )

    async def _arun(self, participant_id: str) -> str:
        """Async version (not implemented, falls back to sync)."""
        return self._run(participant_id)
