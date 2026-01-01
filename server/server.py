"""
Game Server - Main HTTP server with hot-reload capability
Run this with: python server.py
"""

from flask import Flask, request, jsonify
import importlib
import sys
import os
from pathlib import Path
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
import traceback

app = Flask(__name__)

# Store loaded game modules and current game state
games = {}
current_game = None
GAMES_DIR = Path(__file__).parent / "games"

class GameReloader(FileSystemEventHandler):
    """Watches the games directory and reloads modules when files change"""
    
    def on_modified(self, event):
        if event.src_path.endswith('.py'):
            game_name = Path(event.src_path).stem
            print(f"🔄 Reloading {game_name}...")
            load_game(game_name)

def load_game(game_name):
    """Load or reload a game module"""
    try:
        module_path = f"games.{game_name}"
        
        # Remove from sys.modules to force reload
        if module_path in sys.modules:
            importlib.reload(sys.modules[module_path])
        else:
            importlib.import_module(module_path)
        
        games[game_name] = sys.modules[module_path]
        print(f"✅ Loaded game: {game_name}")
        return True
    except Exception as e:
        print(f"❌ Error loading {game_name}: {e}")
        traceback.print_exc()
        return False

def load_all_games():
    """Load all game modules from the games directory"""
    if not GAMES_DIR.exists():
        GAMES_DIR.mkdir()
        print(f"Created games directory: {GAMES_DIR}")
    
    for file in GAMES_DIR.glob("*.py"):
        if file.name != "__init__.py":
            load_game(file.stem)

def format_error_message(game_name, function_name, error):
    """Format a helpful error message for debugging"""
    error_type = type(error).__name__
    error_msg = str(error)
    tb = traceback.format_exc()
    
    return {
        "error": f"Error in {game_name}.{function_name}()",
        "type": error_type,
        "message": error_msg,
        "traceback": tb,
        "help": f"Check games/{game_name}.py - look at the {function_name}() function"
    }

@app.route('/games', methods=['GET'])
def list_games():
    """List all available games"""
    return jsonify({
        "games": list(games.keys()),
        "current_game": current_game
    })

@app.route('/game/<game_name>/start', methods=['POST'])
def start_game(game_name):
    """Start a new game"""
    global current_game
    
    if game_name not in games:
        return jsonify({
            "error": f"Game '{game_name}' not found",
            "available_games": list(games.keys())
        }), 404
    
    try:
        game_module = games[game_name]
        
        # Check if start function exists
        if not hasattr(game_module, 'start'):
            return jsonify({
                "error": f"Game '{game_name}' is missing start() function",
                "help": f"Add a 'def start():' function to games/{game_name}.py"
            }), 500
        
        result = game_module.start()
        current_game = game_name
        
        # Validate return type
        if not isinstance(result, str):
            return jsonify({
                "error": f"start() must return a string, got {type(result).__name__}",
                "returned_value": str(result),
                "help": f"In games/{game_name}.py, make sure start() returns a string message"
            }), 500
        
        return jsonify({
            "message": result,
            "game": game_name
        })
        
    except Exception as e:
        print(f"❌ Error in {game_name}.start():")
        traceback.print_exc()
        return jsonify(format_error_message(game_name, "start", e)), 500

@app.route('/game/<game_name>/message', methods=['POST'])
def send_message(game_name):
    """Send a message to an ongoing game"""
    
    if game_name not in games:
        return jsonify({
            "error": f"Game '{game_name}' not found",
            "available_games": list(games.keys())
        }), 404
    
    try:
        data = request.get_json()
        
        if data is None:
            return jsonify({
                "error": "No JSON data provided",
                "help": "Send a POST request with Content-Type: application/json"
            }), 400
        
        user_input = data.get('input', '')
        
        game_module = games[game_name]
        
        # Check if message function exists
        if not hasattr(game_module, 'message'):
            return jsonify({
                "error": f"Game '{game_name}' is missing message() function",
                "help": f"Add a 'def message(user_input):' function to games/{game_name}.py"
            }), 500
        
        result = game_module.message(user_input)
        
        # Validate return type
        if not isinstance(result, str):
            return jsonify({
                "error": f"message() must return a string, got {type(result).__name__}",
                "returned_value": str(result),
                "help": f"In games/{game_name}.py, make sure message() returns a string message"
            }), 500
        
        return jsonify({
            "message": result,
            "game": game_name
        })
        
    except Exception as e:
        print(f"❌ Error in {game_name}.message():")
        traceback.print_exc()
        return jsonify(format_error_message(game_name, "message", e)), 500

@app.route('/reset', methods=['POST'])
def reset_game():
    """Reset the current game"""
    global current_game
    old_game = current_game
    current_game = None
    return jsonify({
        "status": "reset",
        "was_playing": old_game
    })

@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint"""
    return jsonify({
        "status": "ok", 
        "games_loaded": len(games),
        "current_game": current_game
    })

if __name__ == '__main__':
    print("🎮 Starting Game Server...")
    
    # Add games directory to Python path
    sys.path.insert(0, str(Path(__file__).parent))
    
    # Load all games
    load_all_games()
    
    # Set up file watcher for hot reload
    event_handler = GameReloader()
    observer = Observer()
    observer.schedule(event_handler, str(GAMES_DIR), recursive=False)
    observer.start()
    
    port = 6000
    print(f"👀 Watching {GAMES_DIR} for changes...")
    print(f"🚀 Server starting on http://0.0.0.0:{port}")
    
    try:
        app.run(host='0.0.0.0', port=port, debug=False)
    finally:
        observer.stop()
        observer.join()
