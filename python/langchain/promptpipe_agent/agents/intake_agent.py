"""Intake agent for conducting intake conversations and building user profiles."""

import os
from typing import Optional

from langchain.agents import AgentExecutor, create_tool_calling_agent
from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder
from langchain_openai import ChatOpenAI

from promptpipe_agent.config import settings
from promptpipe_agent.models.schemas import MessageRole
from promptpipe_agent.models.state_manager import StateManager
from promptpipe_agent.tools.profile_save_tool import ProfileSaveTool
from promptpipe_agent.tools.prompt_generator_tool import PromptGeneratorTool
from promptpipe_agent.tools.scheduler_tool import SchedulerTool
from promptpipe_agent.tools.state_transition_tool import StateTransitionTool


class IntakeAgent:
    """Intake agent for conducting intake conversations."""

    def __init__(
        self,
        state_manager: StateManager,
        llm: Optional[ChatOpenAI] = None,
        system_prompt_file: Optional[str] = None,
    ):
        """Initialize the intake agent.

        Args:
            state_manager: State manager for conversation state
            llm: Language model (if None, creates default)
            system_prompt_file: Path to system prompt file (if None, uses default)
        """
        self.state_manager = state_manager

        # Initialize LLM
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
            system_prompt_file or settings.intake_bot_prompt_file
        )
        self.system_prompt = self._load_system_prompt()

        # Initialize tools
        self.tools = self._create_tools()

        # Create agent
        self.agent = self._create_agent()

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
                "You are an intake bot that helps users personalize their habit-building experience. "
                "Ask questions one at a time to collect their preferences, then save their profile."
            )

    def _create_tools(self) -> list:
        """Create the tools available to the intake agent."""
        return [
            StateTransitionTool(state_manager=self.state_manager),
            ProfileSaveTool(state_manager=self.state_manager),
            SchedulerTool(state_manager=self.state_manager),
            PromptGeneratorTool(state_manager=self.state_manager),
        ]

    def _create_agent(self) -> AgentExecutor:
        """Create the LangChain agent executor."""
        prompt = ChatPromptTemplate.from_messages(
            [
                ("system", self.system_prompt),
                MessagesPlaceholder(variable_name="chat_history", optional=True),
                ("human", "{input}"),
                MessagesPlaceholder(variable_name="agent_scratchpad"),
            ]
        )

        agent = create_tool_calling_agent(self.llm, self.tools, prompt)
        return AgentExecutor(agent=agent, tools=self.tools, verbose=settings.debug)

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

        # Get user profile for context
        profile = self.state_manager.get_user_profile(participant_id)
        profile_context = ""
        if profile:
            profile_context = f"\n\nCurrent Profile:\n{profile.to_context_string()}"

        # Convert history to LangChain format (limit if needed)
        chat_history = []
        messages = history.messages
        if settings.chat_history_limit > 0:
            messages = messages[-settings.chat_history_limit :]

        for msg in messages:
            if msg.role == MessageRole.USER:
                chat_history.append(("human", msg.content))
            elif msg.role == MessageRole.ASSISTANT:
                chat_history.append(("ai", msg.content))

        # Add user message to history
        self.state_manager.add_message(participant_id, MessageRole.USER.value, user_message)

        # Enhance input with profile context
        enhanced_input = user_message
        if profile_context:
            enhanced_input = f"{user_message}{profile_context}"

        # Process message through agent
        try:
            result = self.agent.invoke(
                {
                    "input": enhanced_input,
                    "chat_history": chat_history,
                }
            )
            response = result["output"]
        except Exception as e:
            response = f"I apologize, but I encountered an error: {str(e)}"

        # Add response to history
        self.state_manager.add_message(
            participant_id, MessageRole.ASSISTANT.value, response
        )

        return response
