"""Coordinator agent for routing conversations and managing overall flow."""

import os
from typing import Optional

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_openai import ChatOpenAI

from promptpipe_agent.config import settings
from promptpipe_agent.models.schemas import MessageRole
from promptpipe_agent.models.state_manager import StateManager
from promptpipe_agent.tools.profile_save_tool import ProfileSaveTool
from promptpipe_agent.tools.prompt_generator_tool import PromptGeneratorTool
from promptpipe_agent.tools.scheduler_tool import SchedulerTool
from promptpipe_agent.tools.state_transition_tool import StateTransitionTool


class CoordinatorAgent:
    """Coordinator agent for managing conversation flow and routing."""

    def __init__(
        self,
        state_manager: StateManager,
        llm: Optional[ChatOpenAI] = None,
        system_prompt_file: Optional[str] = None,
    ):
        """Initialize the coordinator agent.

        Args:
            state_manager: State manager for conversation state
            llm: Language model (if None, creates default)
            system_prompt_file: Path to system prompt file (if None, uses default)
        """
        self.state_manager = state_manager

        # Initialize LLM with tool binding
        if llm is None:
            self.llm = ChatOpenAI(
                model=settings.openai_model,
                temperature=settings.openai_temperature,
                api_key=settings.openai_api_key,
            )
        else:
            self.llm = llm

        # Load system prompt
        self.system_prompt_file = (
            system_prompt_file or settings.coordinator_prompt_file
        )
        self.system_prompt = self._load_system_prompt()

        # Initialize tools
        self.tools = self._create_tools()
        
        # Bind tools to the LLM
        self.llm_with_tools = self.llm.bind_tools(self.tools)

    def _load_system_prompt(self) -> str:
        """Load the system prompt from file."""
        prompt_file = self.system_prompt_file
        if not os.path.isabs(prompt_file):
            # Make path relative to project root
            base_dir = os.path.dirname(
                os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
            )
            prompt_file = os.path.join(base_dir, prompt_file)

        if os.path.exists(prompt_file):
            with open(prompt_file, "r") as f:
                return f.read().strip()
        else:
            # Fallback prompt
            return (
                "You are a helpful coordinator assistant for a habit-building conversation system. "
                "You can help users with general questions, route them to specialized modules, "
                "and use tools to manage their experience."
            )

    def _create_tools(self) -> list:
        """Create the tools available to the coordinator."""
        tools = []
        
        # Create tool wrappers for LangChain
        state_transition_tool = StateTransitionTool(state_manager=self.state_manager)
        profile_save_tool = ProfileSaveTool(state_manager=self.state_manager)
        scheduler_tool = SchedulerTool(state_manager=self.state_manager)
        prompt_generator_tool = PromptGeneratorTool(state_manager=self.state_manager)
        
        # Convert to LangChain tools (simplified)
        # Note: In a production environment, we'd properly wrap these as LangChain tools
        # For now, we'll handle tool calling manually
        self.state_transition = state_transition_tool
        self.profile_save = profile_save_tool
        self.scheduler = scheduler_tool
        self.prompt_generator = prompt_generator_tool
        
        return []  # We'll handle tools manually for now

    def process_message(
        self, participant_id: str, user_message: str
    ) -> str:
        """Process a user message and return the response.

        Args:
            participant_id: The participant ID
            user_message: The user's message

        Returns:
            The agent's response
        """
        # Get conversation history
        history = self.state_manager.get_conversation_history(participant_id)

        # Build messages
        messages = [SystemMessage(content=self.system_prompt)]
        
        # Add conversation history (limit if needed)
        hist_messages = history.messages
        if settings.chat_history_limit > 0:
            hist_messages = hist_messages[-settings.chat_history_limit :]

        for msg in hist_messages:
            if msg.role == MessageRole.USER:
                messages.append(HumanMessage(content=msg.content))
            elif msg.role == MessageRole.ASSISTANT:
                from langchain_core.messages import AIMessage
                messages.append(AIMessage(content=msg.content))

        # Add current user message
        messages.append(HumanMessage(content=user_message))

        # Add user message to history
        self.state_manager.add_message(participant_id, MessageRole.USER.value, user_message)

        # Process message through LLM
        try:
            response = self.llm.invoke(messages)
            response_text = response.content
        except Exception as e:
            response_text = f"I apologize, but I encountered an error: {str(e)}"

        # Add response to history
        self.state_manager.add_message(
            participant_id, MessageRole.ASSISTANT.value, response_text
        )

        return response_text
