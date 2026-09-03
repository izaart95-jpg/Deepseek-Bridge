package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runChat is the interactive CLI client (port of interactive_chat.py).
func runChat() {
	token := os.Getenv("DEEPSEEK_TOKEN")
	if token == "" {
		fmt.Println("Error: DEEPSEEK_TOKEN environment variable not set")
		fmt.Println("Run this command to set your token:")
		fmt.Println("  export DEEPSEEK_TOKEN='your-token-here'")
		os.Exit(1)
	}

	clearScreen()
	printHeader()
	fmt.Println("Initializing DeepSeek API...")
	api, err := NewDeepSeekAPI(token)
	if err != nil {
		fmt.Printf("Fatal Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("API client initialized successfully!")

	// choose mode
	mode := ""
	scanner := bufio.NewScanner(os.Stdin)
	for mode != "1" && mode != "2" {
		fmt.Println("SELECT MODE:")
		fmt.Println("  1. Threaded Mode - Conversations maintain context")
		fmt.Println("  2. Direct Mode   - Each message is independent")
		fmt.Print("\nEnter choice (1 or 2): ")
		if !scanner.Scan() {
			return
		}
		mode = strings.TrimSpace(scanner.Text())
		if mode != "1" && mode != "2" {
			fmt.Println("Invalid choice. Please enter 1 or 2.")
		}
	}
	modeName := "Threaded"
	if mode == "2" {
		modeName = "Direct"
	}
	fmt.Printf("\n%s mode selected!\n\n", modeName)

	// create initial session
	fmt.Println("Creating chat session...")
	ctx := context.Background()
	sessionID, err := api.CreateChatSession(ctx)
	if err != nil {
		fmt.Printf("Fatal Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Session created: %s...\n\n", short(sessionID))

	parentID := ""
	messageCount := 0

	printHelp()
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Start chatting!")
	fmt.Println(strings.Repeat("=", 60))

	for {
		modeIndicator := "Thread"
		if mode == "2" {
			modeIndicator = "Direct"
		}
		fmt.Printf("\n[%s] You (Type your message, press Enter twice to send, or /command):\n", modeIndicator)
		fmt.Println(strings.Repeat("-", 40))

		var lines []string
		var cmd string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "/") {
				cmd = line
				break
			}
			if line == "" {
				break
			}
			lines = append(lines, line)
		}
		userInput := cmd
		if userInput == "" {
			userInput = strings.Join(lines, " ")
		}

		if strings.HasPrefix(userInput, "/") {
			command := strings.ToLower(userInput)
			switch {
			case command == "/exit":
				fmt.Println("\nGoodbye!")
				return
			case command == "/help":
				printHelp()
				continue
			case command == "/clear":
				clearScreen()
				printHeader()
				fmt.Printf("Current mode: %s\n", modeName)
				fmt.Printf("Session ID: %s...\n", short(sessionID))
				if mode == "1" && parentID != "" {
					fmt.Printf("Last message ID: %s...\n", short(parentID))
				}
				fmt.Println()
				continue
			case command == "/new":
				fmt.Println("\nCreating new chat session...")
				sessionID, err = api.CreateChatSession(ctx)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}
				parentID = ""
				messageCount = 0
				fmt.Printf("New session created: %s...\n\n", short(sessionID))
				continue
			case command == "/mode":
				fmt.Println("\nSwitch mode:")
				fmt.Println("  1. Threaded Mode")
				fmt.Println("  2. Direct Mode")
				fmt.Print("Enter choice (1 or 2): ")
				if !scanner.Scan() {
					return
				}
				newMode := strings.TrimSpace(scanner.Text())
				if newMode == "1" || newMode == "2" {
					if mode == "1" && newMode == "2" {
						parentID = ""
						fmt.Println("Switched to Direct mode (thread reset)")
					} else {
						modeName = "Threaded"
						if newMode == "2" {
							modeName = "Direct"
						}
						fmt.Printf("Switched to %s mode\n", modeName)
					}
					mode = newMode
				} else {
					fmt.Println("Invalid choice")
				}
				continue
			case command == "/session":
				fmt.Printf("\nSession Info:\n")
				fmt.Printf("  Session ID: %s\n", sessionID)
				fmt.Printf("  Mode: %s\n", modeName)
				fmt.Printf("  Messages: %d\n", messageCount)
				if mode == "1" && parentID != "" {
					fmt.Printf("  Last message ID: %s\n", parentID)
				}
				fmt.Println()
				continue
			default:
				fmt.Printf("Unknown command: %s\n", command)
				continue
			}
		}

		if userInput == "" {
			continue
		}

		messageCount++
		fmt.Print("\nDeepSeek: ")

		params := ChatParams{
			ChatSessionID:   sessionID,
			Prompt:          userInput,
			ThinkingEnabled: true,
			SearchEnabled:   false,
		}
		if mode == "1" && parentID != "" {
			params.ParentMessageID = parentID
			if messageCount > 1 {
				fmt.Print("\n[Thread continuation - replying to previous message]\n")
			}
		}

		var fullResponse string
		var lastMessageID any
		err := api.ChatCompletion(ctx, params, func(ch Chunk) bool {
			if ch.Type == "content" && ch.Content != "" {
				fmt.Print(ch.Content)
				fullResponse += ch.Content
			}
			if ch.Type == "ready" {
				if ch.ResponseMessageID != nil {
					lastMessageID = ch.ResponseMessageID
				} else if ch.RequestMessageID != nil {
					lastMessageID = ch.RequestMessageID
				}
			}
			return false
		})

		fmt.Println()
		if err != nil {
			fmt.Printf("Error getting response: %v\n", err)
			fmt.Println("You can try again or use /new to start fresh.")
			continue
		}

		if mode == "1" && lastMessageID != nil {
			parentID = fmt.Sprint(lastMessageID)
			fmt.Printf("\n[Thread updated - can reply to this message]\n")
		}
		fmt.Printf("\n[Response complete - ~%d words]\n\n", len(strings.Fields(fullResponse)))
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func clearScreen() {
	cmd := exec.Command("clear")
	if os.Getenv("OS") == "Windows_NT" {
		cmd = exec.Command("cls")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func printHeader() {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("%s\n", center("DEEPSEEK CHAT CLIENT", 60))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
}

func center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	left := (width - len(s)) / 2
	return strings.Repeat(" ", left) + s
}

func printHelp() {
	fmt.Println("\nCommands:")
	fmt.Println("  /help     - Show this help")
	fmt.Println("  /clear    - Clear screen")
	fmt.Println("  /new      - Start new chat session")
	fmt.Println("  /mode     - Switch between threaded/direct mode")
	fmt.Println("  /exit     - Exit program")
	fmt.Println("  /session  - Show current session info")
	fmt.Println()
}
