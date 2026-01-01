import random

words = [
    "APPLE", "BEACH", "CHAIR", "DANCE", "EARTH",
    "FLAME", "GRAPE", "HEART", "IMAGE", "JUICE",
    "LEMON", "MUSIC", "OCEAN", "PIANO", "QUEEN",
    "ROBOT", "SMILE", "TIGER", "WATER", "YOUTH"
]

secret_word = None
number_of_guesses = 0
maximum_guesses = 6
guesses = []

def start():
    global secret_word, number_of_guesses, guesses
    secret_word = random.choice(words)
    number_of_guesses = 0
    guesses = []
    return f"Guess the 5-letter word! You have {maximum_guesses} guesses."

def message(user_input):
    global number_of_guesses, guesses
    
    guess = user_input.strip().upper()
    
    if len(guess) != 5:
        return "Please enter a 5-letter word!"
    
    if not guess.isalpha():
        return "Please use only letters!"
    
    number_of_guesses += 1
    guesses.append(guess)
    
    # Build a list of letters we haven't matched yet
    letters = list(secret_word)
    colours = []

    # First pass: find exact matches (green)
    for i in range(5):
        if guess[i] == secret_word[i]:
            colours.append("🟩")
            letters.remove(guess[i])
        else:
            colours.append("⬜")

    # Second pass: fill in yellow or grey
    for i in range(5):
        if colours[i] == "⬜":
            if guess[i] in letters:
                colours[i] = "🟨"
                letters.remove(guess[i])

    # Build the result string
    result = guess + "\n" + "".join(colours)
    
    if guess == secret_word:
        result += f"\nCorrect! You found '{secret_word}' in {number_of_guesses} guesses!"
    
    if number_of_guesses >= maximum_guesses:
        result += f"\nGame over! The word was '{secret_word}'."

    return result
