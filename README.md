# Mattermost Game Bot Setup

## Overview

This bot proxies game interactions between Mattermost and the Python game server running on your daughter's machine via Tailscale.

## Architecture

```
Mattermost ←→ Go Bot (your server) ←→ Python Game Server (Tailscale)
```

## Environment Variables

```bash
export MATTERMOST_URL="https://your-mattermost.com"
export BOT_TOKEN="your-bot-token-here"
export GAME_SERVER_URL="http://100.x.x.x:8000"  # Tailscale IP
export LISTEN_ADDR=":8080"  # Optional, defaults to :8080
```

## Running the Bot

```bash
go run bot.go
```

Or build and run:

```bash
go build -o gamebot bot.go
./gamebot
```

## Mattermost Configuration

### 1. Create a Bot Account

1. Go to **System Console** → **Integrations** → **Bot Accounts**
2. Click **Add Bot Account**
3. Fill in:
   - Username: `gamebot`
   - Display Name: `Game Bot`
   - Description: `Interactive game bot`
4. Save and copy the **Access Token** - this is your `BOT_TOKEN`

### 2. Create Slash Command

1. Go to **Integrations** → **Slash Commands**
2. Click **Add Slash Command**
3. Fill in:
   - Title: `Game`
   - Command Trigger Word: `game`
   - Request URL: `https://your-bot-server.com/game`
   - Request Method: `POST`
   - Autocomplete: ✓ Enable
   - Autocomplete Hint: `<gamename>`
   - Autocomplete Description: `Start a game (e.g., number, wordle)`

### 3. Enable Outgoing Webhooks (Optional)

If you want the bot to respond to regular messages in channels:

1. Go to **Integrations** → **Outgoing Webhooks**
2. Click **Add Outgoing Webhook**
3. Fill in:
   - Title: `Game Bot Webhook`
   - Content Type: `application/x-www-form-urlencoded` (default)
   - Callback URLs: `https://your-bot-server.com/webhook`
   - Channel: Select channels where games can be played
   - Trigger Words: Leave empty to trigger on all messages (or use trigger words if you want to limit it)

**Note:** The webhook approach means the bot will see all messages in configured channels. You might want to:
- Use trigger words to limit when the webhook fires
- Check that the user isn't the bot itself to avoid loops
- Only respond when there's an active game session

## Usage

### Start a Game

```
/game number
```

The bot will:
1. Call the Python server to start the game
2. Store the session for that channel
3. Post the initial message

### Play the Game

Just type your moves as regular messages in the channel:

```
50
```

The bot will:
1. Send the input to the Python server
2. Post the response back to the channel

### Switch Games

Just start a new game:

```
/game wordle
```

This automatically switches to the new game.

## Bot Behavior

- **One game per channel** - Each channel can have one active game at a time
- **Session management** - The bot tracks which game is active in each channel
- **Error handling** - Shows helpful error messages from the Python server
- **Ephemeral responses** - Command usage errors only visible to the user
- **Channel responses** - Game messages visible to everyone

## Debugging

### Check if bot is running:
```bash
curl http://localhost:8080/health
```

### Check if Python server is reachable:
```bash
curl http://100.x.x.x:8000/health
```

### Test game start manually:
```bash
curl -X POST http://100.x.x.x:8000/game/number/start
```

## Architecture Notes

- The Go bot is **stateless** except for tracking active games per channel
- All game state lives in the Python server
- The Python server runs one game at a time globally
- Multiple channels talking to the same Python server will share the same game instance

If you need **multiple simultaneous games** (one per channel), you'd need to either:
1. Run multiple Python servers (one per channel)
2. Modify the Python server to track state per session ID
3. Have the Go bot pass state back and forth (your original architecture)

For a single-user learning environment, the current approach is simplest!

## Security Considerations

- The bot token should be kept secret
- Consider adding request validation (token checking) on the webhook endpoint
- The Python server should only be accessible via Tailscale, not the public internet
- You might want to restrict which channels can use the bot

## Future Enhancements

- Add a `/game stop` command to end the current game
- Add a `/game list` command to show available games
- Track game statistics (wins, attempts, etc.)
- Support multiple games per channel by including a session ID
- Add user-specific game instances instead of channel-wide games
