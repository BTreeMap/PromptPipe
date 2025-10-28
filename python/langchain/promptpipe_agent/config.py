"""Configuration management for PromptPipe Agent."""

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # OpenAI Configuration
    openai_api_key: str = Field(..., description="OpenAI API key")
    openai_model: str = Field(default="gpt-4o-mini", description="OpenAI model to use")
    openai_temperature: float = Field(default=0.1, description="OpenAI temperature")

    # State Directory
    promptpipe_state_dir: str = Field(
        default="/var/lib/promptpipe", description="PromptPipe state directory"
    )

    # Prompt Files (relative to project root or absolute paths)
    intake_bot_prompt_file: str = Field(
        default="../../prompts/intake_bot_system.txt",
        description="Path to intake bot system prompt",
    )
    coordinator_prompt_file: str = Field(
        default="../../prompts/conversation_system_3bot.txt",
        description="Path to coordinator system prompt",
    )
    feedback_tracker_prompt_file: str = Field(
        default="../../prompts/feedback_tracker_system.txt",
        description="Path to feedback tracker system prompt",
    )
    prompt_generator_prompt_file: str = Field(
        default="../../prompts/prompt_generator_system.txt",
        description="Path to prompt generator system prompt",
    )

    # Timeouts
    feedback_initial_timeout: str = Field(
        default="15m", description="Initial feedback timeout (e.g., 15m)"
    )
    feedback_followup_delay: str = Field(
        default="3h", description="Follow-up feedback delay (e.g., 3h)"
    )

    # Chat History
    chat_history_limit: int = Field(
        default=-1,
        description="Limit for chat history (-1: unlimited, 0: none, N: last N messages)",
    )

    # API Configuration
    api_host: str = Field(default="0.0.0.0", description="API host")
    api_port: int = Field(default=8001, description="API port")

    # Debug Mode
    debug: bool = Field(default=False, description="Enable debug mode")


# Global settings instance
settings = Settings()
