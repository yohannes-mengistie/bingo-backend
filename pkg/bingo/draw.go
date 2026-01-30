package bingo

import (
	"crypto/rand"
	"math/big"

	"github.com/bingo/backend/internal/domain"
)

// DrawNumber draws a random number for a given letter
// Returns the number and ensures it hasn't been drawn before
func DrawNumber(letter string, drawnNumbers []int) (int, error) {
	var min, max int

	switch letter {
	case string(domain.BingoLetterB):
		min, max = domain.BingoNumberMinB, domain.BingoNumberMaxB
	case string(domain.BingoLetterI):
		min, max = domain.BingoNumberMinI, domain.BingoNumberMaxI
	case string(domain.BingoLetterN):
		min, max = domain.BingoNumberMinN, domain.BingoNumberMaxN
	case string(domain.BingoLetterG):
		min, max = domain.BingoNumberMinG, domain.BingoNumberMaxG
	case string(domain.BingoLetterO):
		min, max = domain.BingoNumberMinO, domain.BingoNumberMaxO
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
	letters := []string{
		string(domain.BingoLetterB),
		string(domain.BingoLetterI),
		string(domain.BingoLetterN),
		string(domain.BingoLetterG),
		string(domain.BingoLetterO),
	}

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
	minDrawn := domain.NumbersPerLetter // Max possible per letter

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

