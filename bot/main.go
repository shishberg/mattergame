package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Config holds bot configuration
type Config struct {
	MattermostURL string
	BotToken      string
	GameServerURL string
	ListenAddr    string
}

// GameSession tracks active games per channel
type GameSession struct {
	GameName string
	Active   bool
}

// Bot handles Mattermost interactions
type Bot struct {
	config   Config
	sessions map[string]*GameSession // channelID -> session
	mu       sync.RWMutex
}

// GameResponse from Python server
type GameResponse struct {
	Message string `json:"message"`
	Game    string `json:"game"`
	Error   string `json:"error"`
	Help    string `json:"help"`
}

// SlashCommandRequest from Mattermost
type SlashCommandRequest struct {
	Token       string `json:"token"`
	TeamID      string `json:"team_id"`
	TeamDomain  string `json:"team_domain"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	Command     string `json:"command"`
	Text        string `json:"text"`
}

// Post to Mattermost
type Post struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}

func main() {
	config := Config{
		MattermostURL: getEnv("MATTERMOST_URL", "https://your-mattermost.com"),
		BotToken:      getEnv("MATTERMOST_BOT_TOKEN", ""),
		GameServerURL: getEnv("GAME_SERVER_URL", "http://localhost:6000"),
		ListenAddr:    getEnv("LISTEN_ADDR", ":6001"),
	}

	if config.BotToken == "" {
		log.Fatal("MATTERMOST_BOT_TOKEN environment variable is required")
	}

	bot := &Bot{
		config:   config,
		sessions: make(map[string]*GameSession),
	}

	http.HandleFunc("/game", bot.handleSlashCommand)
	http.HandleFunc("/webhook", bot.handleWebhook)
	http.HandleFunc("/health", handleHealth)

	log.Printf("🤖 Bot starting on %s", config.ListenAddr)
	log.Printf("🎮 Game server: %s", config.GameServerURL)
	log.Fatal(http.ListenAndServe(config.ListenAddr, nil))
}

// handleSlashCommand handles /game commands
func (b *Bot) handleSlashCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Extract command parameters from form
	cmd := SlashCommandRequest{
		Token:       r.FormValue("token"),
		TeamID:      r.FormValue("team_id"),
		TeamDomain:  r.FormValue("team_domain"),
		ChannelID:   r.FormValue("channel_id"),
		ChannelName: r.FormValue("channel_name"),
		UserID:      r.FormValue("user_id"),
		UserName:    r.FormValue("user_name"),
		Command:     r.FormValue("command"),
		Text:        r.FormValue("text"),
	}

	// Parse command: /game <gamename>
	gameName := strings.TrimSpace(cmd.Text)
	if gameName == "" {
		b.respondEphemeral(w, "Usage: /game <gamename>\nExample: /game number")
		return
	}

	// Start the game
	response, err := b.startGame(gameName)
	if err != nil {
		b.respondEphemeral(w, fmt.Sprintf("❌ Error starting game: %v", err))
		return
	}

	if response.Error != "" {
		msg := fmt.Sprintf("❌ %s", response.Error)
		if response.Help != "" {
			msg += fmt.Sprintf("\n💡 %s", response.Help)
		}
		b.respondEphemeral(w, msg)
		return
	}

	// Store session
	b.mu.Lock()
	b.sessions[cmd.ChannelID] = &GameSession{
		GameName: gameName,
		Active:   true,
	}
	b.mu.Unlock()

	// Respond in channel
	b.respondInChannel(w, fmt.Sprintf("**Starting game: %s**\n\n%s", gameName, response.Message))
}

// handleWebhook handles messages in channels with active games
func (b *Bot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data (outgoing webhooks send application/x-www-form-urlencoded by default)
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Extract relevant fields
	channelID := r.FormValue("channel_id")
	// userID := r.FormValue("user_id")
	text := r.FormValue("text")
	// token := r.FormValue("token")

	// Check if there's an active game in this channel
	b.mu.RLock()
	session := b.sessions[channelID]
	b.mu.RUnlock()

	if session == nil || !session.Active {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Don't respond to bot's own messages
	// (You may need to check if userID matches bot's user ID)

	// Process the move
	response, err := b.processMove(session.GameName, text)
	if err != nil {
		b.postMessage(channelID, fmt.Sprintf("❌ Error: %v", err))
		w.WriteHeader(http.StatusOK)
		return
	}

	if response.Error != "" {
		msg := fmt.Sprintf("❌ %s", response.Error)
		if response.Help != "" {
			msg += fmt.Sprintf("\n💡 %s", response.Help)
		}
		b.postMessage(channelID, msg)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Post response
	b.postMessage(channelID, response.Message)
	w.WriteHeader(http.StatusOK)
}

// startGame calls the Python game server to start a game
func (b *Bot) startGame(gameName string) (*GameResponse, error) {
	url := fmt.Sprintf("%s/game/%s/start", b.config.GameServerURL, gameName)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to game server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var gameResp GameResponse
	if err := json.Unmarshal(body, &gameResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &gameResp, nil
}

// processMove calls the Python game server to process a move
func (b *Bot) processMove(gameName, userInput string) (*GameResponse, error) {
	url := fmt.Sprintf("%s/game/%s/move", b.config.GameServerURL, gameName)

	payload := map[string]string{"input": userInput}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to game server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var gameResp GameResponse
	if err := json.Unmarshal(body, &gameResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &gameResp, nil
}

// postMessage posts a message to a channel
func (b *Bot) postMessage(channelID, message string) error {
	url := fmt.Sprintf("%s/api/v4/posts", b.config.MattermostURL)

	post := Post{
		ChannelID: channelID,
		Message:   message,
	}

	jsonData, err := json.Marshal(post)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.config.BotToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to post message: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// respondEphemeral sends an ephemeral response (only visible to user)
func (b *Bot) respondEphemeral(w http.ResponseWriter, message string) {
	response := map[string]interface{}{
		"response_type": "ephemeral",
		"text":          message,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// respondInChannel sends a response visible to everyone
func (b *Bot) respondInChannel(w http.ResponseWriter, message string) {
	response := map[string]interface{}{
		"response_type": "in_channel",
		"text":          message,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
