package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// LLMConfig holds configuration for the LLM service
type LLMConfig struct {
	OpenRouterAPIKey string
	Model            string
}

// OpenRouterRequest represents a request to OpenRouter API
type OpenRouterRequest struct {
	Model    string          `json:"model"`
	Messages []OpenRouterMsg `json:"messages"`
}

// OpenRouterMsg represents a message in the OpenRouter API
type OpenRouterMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenRouterResponse represents a response from OpenRouter API
type OpenRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// SourceResponse from Python server
type SourceResponse struct {
	Game   string `json:"game"`
	Source string `json:"source"`
	Error  string `json:"error"`
}

// MattermostPostsResponse represents the posts response from Mattermost API
type MattermostPostsResponse struct {
	Order []string                  `json:"order"`
	Posts map[string]MattermostPost `json:"posts"`
}

// MattermostPost represents a single post from Mattermost API
type MattermostPost struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	UserID   string `json:"user_id"`
	CreateAt int64  `json:"create_at"`
}

// provideHelp handles requests for checking the game source and getting LLM advice
func (b *Bot) provideHelp(channelID, userQuestion string) (string, error) {
	// Check if there's an active game in this channel
	b.mu.RLock()
	session := b.sessions[channelID]
	b.mu.RUnlock()

	if session == nil || !session.Active {
		return "", fmt.Errorf("No active game in this channel. Start a game with `/game <gamename>` first!")
	}

	// Get the game source code
	gameSource, err := b.getGameSource(session.GameName)
	if err != nil {
		log.Printf("Error getting game source: %v", err)
		return "", fmt.Errorf("Couldn't get game source: %v", err)
	}

	// Get recent channel messages
	recentMessages, err := b.getRecentMessages(channelID, 5)
	if err != nil {
		log.Printf("Error getting recent messages: %v", err)
		// Continue without context if we can't get messages
		recentMessages = []string{}
	}

	// Get LLM response
	llmResponse, err := b.getLLMResponse(session.GameName, gameSource, recentMessages, userQuestion)
	if err != nil {
		log.Printf("Error getting LLM response: %v", err)
		return "", fmt.Errorf("Error from AI assistant: %v", err)
	}

	return fmt.Sprintf("**🤖 Coding Help**\n\n%s", llmResponse), nil
}

// getGameSource fetches the source code for a game from the Python server
func (b *Bot) getGameSource(gameName string) (string, error) {
	url := fmt.Sprintf("%s/game/%s/source", b.config.GameServerURL, gameName)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to connect to game server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var sourceResp SourceResponse
	if err := json.Unmarshal(body, &sourceResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if sourceResp.Error != "" {
		return "", fmt.Errorf("%s", sourceResp.Error)
	}

	return sourceResp.Source, nil
}

// getRecentMessages fetches the last N messages from a channel
func (b *Bot) getRecentMessages(channelID string, count int) ([]string, error) {
	url := fmt.Sprintf("%s/api/v4/channels/%s/posts?per_page=%d", b.config.MattermostURL, channelID, count)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+b.config.BotToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch messages: %d - %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var postsResp MattermostPostsResponse
	if err := json.Unmarshal(body, &postsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract messages in order (order is newest first)
	messages := make([]string, 0, len(postsResp.Order))
	for _, postID := range postsResp.Order {
		if post, ok := postsResp.Posts[postID]; ok {
			if post.Message != "" {
				messages = append(messages, post.Message)
			}
		}
	}

	// Reverse to get oldest first (for better context flow)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// getLLMResponse calls OpenRouter to get coding assistance
func (b *Bot) getLLMResponse(gameName, gameSource string, recentMessages []string, userQuestion string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	// Build context from recent messages
	var contextMessages string
	if len(recentMessages) > 0 {
		contextMessages = "\n\n**Recent channel messages:**\n"
		for _, msg := range recentMessages {
			contextMessages += fmt.Sprintf("- %s\n", msg)
		}
	}

	if userQuestion == "" {
		userQuestion = fmt.Sprintf(`\n**Student's Question:**\n"%s"\n`, userQuestion)
	}

	// Build the system prompt
	systemPrompt := `You are a friendly coding tutor helping an absolute beginner learn to program Python through a simple game.

Your role is to:
1. Show them what changes to make to their code for the fixes or improvements they asked for
2. Explain the changes in the simplest possible terms
3. Keep both the code and the explanation as short and simple as possible

Remember: The student is a complete beginner. Assume they know nothing about programming.`

	// Build the user message with context
	userMessage := fmt.Sprintf(`**Current Game: %s**

Here is the game's Python code:
%s
%s

The game is running in a framework that calls the 'start' function to begin a game, and the 'message' function to handle messages from the user.
%s
Please help this beginner understand their question. Remember to:
- Show them what changes to make to their code
- Keep it short and simple
- Encourage them to experiment and learn`,
		gameName,
		"```python\n"+gameSource+"\n```",
		contextMessages,
		userQuestion)

	// Build the request
	reqBody := OpenRouterRequest{
		Model: "google/gemini-2.5-flash",
		Messages: []OpenRouterMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/shishberg/mattergame")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var llmResp OpenRouterResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if llmResp.Error != nil {
		return "", fmt.Errorf("OpenRouter error: %s", llmResp.Error.Message)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	return llmResp.Choices[0].Message.Content, nil
}
