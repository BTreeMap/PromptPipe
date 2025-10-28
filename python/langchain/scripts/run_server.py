"""Manual test script for the FastAPI application."""

import os
import sys

# Add the parent directory to the path so we can import our modules
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from promptpipe_agent.api.main import app

if __name__ == "__main__":
    import uvicorn
    
    print("Starting PromptPipe Agent API server...")
    print("API documentation available at: http://localhost:8001/docs")
    print("Health check: http://localhost:8001/health")
    
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8001,
        log_level="info",
    )
