import os
import sys
from pathlib import Path

# Import from api.py
from api import DeepSeekAPI

def main():
    # Get token from environment
    token = os.environ.get("DEEPSEEK_TOKEN")
    if not token:
        print("❌ Error: DEEPSEEK_TOKEN environment variable not set")
        print("Run: export DEEPSEEK_TOKEN='your-token-here'")
        sys.exit(1)

    print(f"✅ Token loaded (length: {len(token)})")

    try:
        # Initialize the API client
        print("\n🔄 Initializing DeepSeek API...")
        api = DeepSeekAPI(token)
        print("✅ API client initialized")

        # Create a chat session
        print("\n🔄 Creating chat session...")
        session_id = api.create_chat_session()
        print(f"✅ Session created: {session_id}")

        # Send a message
        print("\n🤔 Sending message: 'Explain what you can do in 2 sentences'")
        print("-" * 60)

        # Stream the response
        response_text = ""
        for chunk in api.chat_completion(
            chat_session_id=session_id,
            prompt="Explain what you can do in 2 sentences",
            thinking_enabled=True,
            search_enabled=False
        ):
            if chunk.get('content'):
                print(chunk['content'], end='', flush=True)
                response_text += chunk['content']

        print("\n" + "-" * 60)
        print("✅ Response complete!")

    except Exception as e:
        print(f"\n❌ Error: {type(e).__name__}: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
