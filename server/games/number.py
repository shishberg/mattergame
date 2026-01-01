import random

secret_number = None
number_of_guesses = 0


def start():
    global secret_number, number_of_guesses
    secret_number = random.randint(1, 100)
    number_of_guesses = 0
    return "I'm thinking of a number between 1 and 100. Can you guess it?"


def message(user_input):
    global number_of_guesses

    guess = int(user_input.strip())

    if guess < 1 or guess > 100:
        return "Your guess should be between 1 and 100!"

    number_of_guesses += 1

    if guess < secret_number:
        return "Higher"
    if guess > secret_number:
        return "Lower"
    if guess == secret_number:
        return f"Correct! You got it in {number_of_guesses} guesses!"
