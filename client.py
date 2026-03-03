import os
import sys
import json
from pathlib import Path
from api import DeepSeekAPI

def clear_screen():
    """Clear the terminal screen"""
    os.system('cls' if os.name == 'nt' else 'clear')

def print_header():
    """Print nice header"""
    print("="*60)
    print("🤖 DEEPSEEK CHAT CLIENT".center(60))
    print("="*60)
    print()

def print_help():
    """Print help information"""
    print("\n📋 Commands:")
    print("  /help     - Show this help")
    print("  /clear    - Clear screen")
    print("  /new      - Start new chat session")
    print("  /mode     - Switch between threaded/direct mode")
    print("  /exit     - Exit program")
    print("  /session  - Show current session info")
    print()

def get_user_input(prompt_text):
    """Get multi-line user input until empty line"""
    print(f"\n{prompt_text} (Type your message, press Enter twice to send, or /command):")
    print("-" * 40)
    
    lines = []
    while True:
        line = input()
        if line.startswith('/'):
            return line
        if line == "":
            break
        lines.append(line)
    
    return " ".join(lines) if lines else ""

def main():
    # Get token from environment
    token = os.environ.get("DEEPSEEK_TOKEN")
    if not token:
        print_header()
        print("❌ Error: DEEPSEEK_TOKEN environment variable not set")
        print("\nRun this command to set your token:")
        print("  export DEEPSEEK_TOKEN='your-token-here'")
        sys.exit(1)

    try:
        # Initialize the API client
        clear_screen()
        print_header()
        print("🔄 Initializing DeepSeek API...")
        api = DeepSeekAPI(token)
        print("✅ API client initialized successfully!\n")

        # Choose mode
        current_mode = None
        while current_mode not in ['1', '2']:
            print("📌 SELECT MODE:")
            print("  1️⃣  Threaded Mode - Conversations maintain context")
            print("  2️⃣  Direct Mode   - Each message is independent")
            current_mode = input("\nEnter choice (1 or 2): ").strip()
            
            if current_mode not in ['1', '2']:
                print("❌ Invalid choice. Please enter 1 or 2.")

        mode_name = "Threaded" if current_mode == '1' else "Direct"
        print(f"\n✅ {mode_name} mode selected!\n")

        # Create initial chat session
        print("🔄 Creating chat session...")
        session_id = api.create_chat_session()
        print(f"✅ Session created: {session_id[:8]}...\n")
        
        # Track conversation thread
        parent_id = None
        message_count = 0
        
        print_help()
        print("\n" + "="*60)
        print("💬 Start chatting!".center(60))
        print("="*60)

        while True:
            # Show mode indicator in prompt
            mode_indicator = "📎 Thread" if current_mode == '1' else "💬 Direct"
            prompt_display = f"[{mode_indicator}] You"
            
            user_input = get_user_input(prompt_display)

            # Handle commands
            if user_input.startswith('/'):
                cmd = user_input.lower()
                
                if cmd == '/exit':
                    print("\n👋 Goodbye!")
                    break
                    
                elif cmd == '/help':
                    print_help()
                    continue
                    
                elif cmd == '/clear':
                    clear_screen()
                    print_header()
                    print(f"📌 Current mode: {mode_name}")
                    print(f"📌 Session ID: {session_id[:8]}...")
                    if current_mode == '1' and parent_id:
                        print(f"📌 Last message ID: {parent_id[:8]}...")
                    print()
                    continue
                    
                elif cmd == '/new':
                    print("\n🔄 Creating new chat session...")
                    session_id = api.create_chat_session()
                    parent_id = None
                    message_count = 0
                    print(f"✅ New session created: {session_id[:8]}...\n")
                    continue
                    
                elif cmd == '/mode':
                    print("\n📌 Switch mode:")
                    print("  1️⃣  Threaded Mode")
                    print("  2️⃣  Direct Mode")
                    new_mode = input("Enter choice (1 or 2): ").strip()
                    
                    if new_mode in ['1', '2']:
                        old_mode = current_mode
                        current_mode = new_mode
                        mode_name = "Threaded" if current_mode == '1' else "Direct"
                        
                        # Reset thread if switching from threaded to direct
                        if old_mode == '1' and current_mode == '2':
                            parent_id = None
                            print("✅ Switched to Direct mode (thread reset)")
                        else:
                            print(f"✅ Switched to {mode_name} mode")
                    else:
                        print("❌ Invalid choice")
                    continue
                    
                elif cmd == '/session':
                    print(f"\n📌 Session Info:")
                    print(f"  • Session ID: {session_id}")
                    print(f"  • Mode: {mode_name}")
                    print(f"  • Messages: {message_count}")
                    if current_mode == '1' and parent_id:
                        print(f"  • Last message ID: {parent_id}")
                    print()
                    continue
                    
                else:
                    print(f"❌ Unknown command: {cmd}")
                    continue

            # Skip empty messages
            if not user_input:
                continue

            # Send message to API
            message_count += 1
            print(f"\n🤖 DeepSeek: ", end='', flush=True)

            try:
                # Prepare completion parameters
                completion_params = {
                    'chat_session_id': session_id,
                    'prompt': user_input,
                    'thinking_enabled': True,
                    'search_enabled': False
                }
                
                # Add parent_message_id only in threaded mode
                if current_mode == '1' and parent_id:
                    completion_params['parent_message_id'] = parent_id
                    if message_count > 1:
                        print(f"\n[Thread continuation - replying to previous message]\n")

                # Stream the response
                full_response = ""
                last_message_id = None
                
                for chunk in api.chat_completion(**completion_params):
                    # Handle different chunk types
                    if chunk.get('type') == 'content' and chunk.get('content'):
                        print(chunk['content'], end='', flush=True)
                        full_response += chunk['content']
                    
                    # Capture message ID for threading
                    if chunk.get('type') == 'ready':
                        if 'response_message_id' in chunk:
                            last_message_id = chunk['response_message_id']
                        elif 'request_message_id' in chunk:
                            last_message_id = chunk['request_message_id']
                    
                    # Also check for message_id in other formats
                    if 'message_id' in chunk:
                        last_message_id = chunk['message_id']
                
                print()  # New line after response
                
                # Update parent_id for threading if we got a message ID
                if current_mode == '1' and last_message_id:
                    parent_id = last_message_id
                    print(f"\n[📎 Thread updated - can reply to this message]")
                
                # Small stats
                words = len(full_response.split())
                print(f"\n[Response complete - ~{words} words]\n")

            except Exception as e:
                print(f"\n❌ Error getting response: {type(e).__name__}: {e}")
                print("You can try again or use /new to start fresh.\n")

    except KeyboardInterrupt:
        print("\n\n👋 Goodbye!")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Fatal Error: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    main()
