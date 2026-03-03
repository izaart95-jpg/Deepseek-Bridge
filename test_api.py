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

        # First message - Start a thread
        print("\n" + "="*60)
        print("🤔 FIRST MESSAGE: 'Tell me about neural networks'")
        print("="*60)
        
        parent_id = None
        for chunk in api.chat_completion(
            chat_session_id=session_id,
            prompt="Tell me about neural networks",
            thinking_enabled=True,
            search_enabled=False
        ):
            # Check different chunk types based on your API's response format
            if chunk.get('type') == 'content' or chunk.get('content'):
                print(chunk['content'], end='', flush=True)
            
            # Capture message ID for threading (adjust based on actual response)
            if chunk.get('type') == 'ready' and 'response_message_id' in chunk:
                parent_id = chunk['response_message_id']
            elif chunk.get('type') == 'complete':
                print("\n" + "-"*60)
                print("✅ First response complete")

        # If we didn't get a message ID from 'ready' event, try to get it from the API
        if not parent_id:
            print("\n⚠️  Could not capture message ID automatically")
            print("Using session ID as parent for next message")
            parent_id = session_id

        # Second message - Follow-up in the same thread
        print("\n" + "="*60)
        print(f"🤔 FOLLOW-UP: 'How do they compare to other ML models?'")
        print(f"📎 Thread parent ID: {parent_id}")
        print("="*60)

        for chunk in api.chat_completion(
            chat_session_id=session_id,
            prompt="How do they compare to other ML models?",
            parent_message_id=parent_id,  # This creates the thread
            thinking_enabled=True,
            search_enabled=False
        ):
            if chunk.get('type') == 'content' or chunk.get('content'):
                print(chunk['content'], end='', flush=True)
            
            if chunk.get('type') == 'complete':
                print("\n" + "-"*60)
                print("✅ Follow-up response complete")

        print("\n🎉 Conversation thread completed successfully!")

    except Exception as e:
        print(f"\n❌ Error: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    main()