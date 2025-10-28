"""Conversation orchestrator for routing messages to appropriate agents."""

from typing import Optional

from langchain_openai import ChatOpenAI

from promptpipe_agent.agents.coordinator_agent import CoordinatorAgent
from promptpipe_agent.agents.feedback_agent import FeedbackAgent
from promptpipe_agent.agents.intake_agent import IntakeAgent
from promptpipe_agent.models.schemas import ConversationState
from promptpipe_agent.models.state_manager import StateManager


class ConversationOrchestrator:
    """Orchestrator that routes messages to the appropriate agent based on state."""

    def __init__(
        self,
        state_manager: StateManager,
        llm: Optional[ChatOpenAI] = None,
    ):
        """Initialize the conversation orchestrator.

        Args:
            state_manager: State manager for conversation state
            llm: Language model to use (if None, each agent creates its own)
        """
        self.state_manager = state_manager

        # Initialize agents
        self.coordinator = CoordinatorAgent(state_manager, llm)
        self.intake = IntakeAgent(state_manager, llm)
        self.feedback = FeedbackAgent(state_manager, llm)

    def process_message(
        self, participant_id: str, user_message: str
    ) -> tuple[str, ConversationState]:
        """Process a user message by routing to the appropriate agent.

        Args:
            participant_id: The participant ID
            user_message: The user's message

        Returns:
            A tuple of (response, current_state)
        """
        # Get current state
        current_state = self.state_manager.get_current_state(participant_id)

        # Default to COORDINATOR if no state set
        if current_state is None:
            current_state = ConversationState.COORDINATOR
            self.state_manager.set_current_state(participant_id, current_state)

        # Route to appropriate agent
        if current_state == ConversationState.INTAKE:
            response = self.intake.process_message(participant_id, user_message)
        elif current_state == ConversationState.FEEDBACK:
            response = self.feedback.process_message(participant_id, user_message)
        else:  # COORDINATOR or CONVERSATION_ACTIVE
            response = self.coordinator.process_message(participant_id, user_message)

        # Get updated state (may have changed via state transition tool)
        updated_state = self.state_manager.get_current_state(participant_id)

        return response, updated_state
