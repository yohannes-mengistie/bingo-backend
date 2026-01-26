package bingo

import (
	"crypto/rand"
	"math/big"
)

// DrawNumber draws a random number for a given letter
// Returns the number and ensures it hasn't been drawn before
func DrawNumber(letter string, drawnNumbers []int) (int, error) {
	var min, max int

	switch letter {
	case "B":
		min, max = 1, 15
	case "I":
		min, max = 16, 30
	case "N":
		min, max = 31, 45
	case "G":
		min, max = 46, 60
	case "O":
		min, max = 61, 75
	default:
		return 0, nil
	}

	// Get available numbers (not yet drawn)
	available := make([]int, 0)
	drawnSet := make(map[int]bool)
	for _, num := range drawnNumbers {
		drawnSet[num] = true
	}

	for num := min; num <= max; num++ {
		if !drawnSet[num] {
			available = append(available, num)
		}
	}

	if len(available) == 0 {
		return 0, nil // All numbers drawn for this letter
	}

	// Select random number from available
	rangeSize := big.NewInt(int64(len(available)))
	randIdx, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return 0, err
	}

	return available[randIdx.Int64()], nil
}

// DrawNextNumber draws the next number in BINGO order
// B -> I -> N -> G -> O, repeating until all numbers are drawn
func DrawNextNumber(drawnNumbers []int) (string, int, error) {
	letters := []string{"B", "I", "N", "G", "O"}

	// Count drawn numbers per letter
	drawnCount := make(map[string]int)
	for _, num := range drawnNumbers {
		letter := GetLetterForNumber(num)
		if letter != "" {
			drawnCount[letter]++
		}
	}

	// Find the letter with the least drawn numbers (round-robin)
	var nextLetter string
	minDrawn := 15 // Max possible per letter

	for _, letter := range letters {
		count := drawnCount[letter]
		if count < minDrawn {
			minDrawn = count
			nextLetter = letter
		}
	}

	// Draw number for the selected letter
	number, err := DrawNumber(nextLetter, drawnNumbers)
	if err != nil {
		return "", 0, err
	}

	return nextLetter, number, nil
}

