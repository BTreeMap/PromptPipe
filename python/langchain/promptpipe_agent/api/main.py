"""FastAPI application for PromptPipe Agent."""

from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware

from promptpipe_agent.agents.orchestrator import ConversationOrchestrator
from promptpipe_agent.config import settings
from promptpipe_agent.models.schemas import (
    HealthResponse,
    ProcessMessageRequest,
    ProcessMessageResponse,
)
from promptpipe_agent.models.state_manager import SQLiteStateManager


# Global orchestrator instance
orchestrator: ConversationOrchestrator = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown events."""
    # Startup: Initialize the orchestrator
    global orchestrator
    state_manager = SQLiteStateManager()
    orchestrator = ConversationOrchestrator(state_manager)
    yield
    # Shutdown: Clean up resources if needed
    pass


# Create FastAPI app
app = FastAPI(
    title="PromptPipe Agent",
    description="LangChain-based agentic conversation flow for PromptPipe",
    version="0.1.0",
    lifespan=lifespan,
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # In production, specify allowed origins
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint."""
    return HealthResponse(status="healthy", version="0.1.0")


@app.post("/process-message", response_model=ProcessMessageResponse)
async def process_message(request: ProcessMessageRequest):
    """Process a user message through the conversation flow.

    This endpoint receives a user message from the Go service and processes it
    through the appropriate agent (Coordinator, Intake, or Feedback) based on
    the current conversation state.

    Args:
        request: The message processing request

    Returns:
        The agent's response along with the current conversation state

    Raises:
        HTTPException: If processing fails
    """
    try:
        # Process the message through the orchestrator
        response, state = orchestrator.process_message(
            request.participant_id, request.message
        )

        return ProcessMessageResponse(
            response=response,
            state=state,
            metadata={
                "participant_id": request.participant_id,
                "phone_number": request.phone_number,
            },
        )

    except Exception as e:
        raise HTTPException(
            status_code=500,
            detail=f"Error processing message: {str(e)}",
        )


@app.get("/")
async def root():
    """Root endpoint with API information."""
    return {
        "name": "PromptPipe Agent",
        "version": "0.1.0",
        "description": "LangChain-based agentic conversation flow",
        "endpoints": {
            "health": "/health",
            "process_message": "/process-message",
            "docs": "/docs",
        },
    }


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        app,
        host=settings.api_host,
        port=settings.api_port,
        log_level="info",
    )
