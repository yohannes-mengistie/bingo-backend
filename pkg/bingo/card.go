package bingo

import (
	"crypto/rand"
	"math/big"
)

// BingoCard represents a 5x5 bingo card
type BingoCard struct {
	ID      int
	Numbers [5][5]int
}

// GenerateCard generates a valid bingo card with numbers in the correct ranges
// B: 1-15, I: 16-30, N: 31-45, G: 46-60, O: 61-75
// Center cell (N column, row 2) is treated as free (0)
func GenerateCard(cardID int) *BingoCard {
	card := &BingoCard{
		ID:      cardID,
		Numbers: [5][5]int{},
	}

	ranges := [5]struct {
		min, max int
	}{
		{1, 15},   // B
		{16, 30},  // I
		{31, 45},  // N
		{46, 60},  // G
		{61, 75},  // O
	}

	// Generate numbers for each column
	for col := 0; col < 5; col++ {
		numbers := generateUniqueNumbers(ranges[col].min, ranges[col].max, 5)
		for row := 0; row < 5; row++ {
			// Center cell (col 2, row 2) is free
			if col == 2 && row == 2 {
				card.Numbers[row][col] = 0
			} else {
				card.Numbers[row][col] = numbers[row]
			}
		}
	}

	return card
}

// GenerateAllCards generates all 100 unique bingo cards
func GenerateAllCards() []*BingoCard {
	cards := make([]*BingoCard, 100)
	for i := 1; i <= 100; i++ {
		cards[i-1] = GenerateCard(i)
	}
	return cards
}

// generateUniqueNumbers generates n unique random numbers in the range [min, max]
func generateUniqueNumbers(min, max, n int) []int {
	numbers := make([]int, 0, n)
	used := make(map[int]bool)

	for len(numbers) < n {
		// Generate random number in range
		rangeSize := big.NewInt(int64(max - min + 1))
		randNum, _ := rand.Int(rand.Reader, rangeSize)
		num := int(randNum.Int64()) + min

		if !used[num] {
			used[num] = true
			numbers = append(numbers, num)
		}
	}

	return numbers
}

// ValidateBingo checks if the marked numbers form a valid bingo
// A valid bingo can be:
// - Any row (5 numbers)
// - Any column (5 numbers)
// - Main diagonal (top-left to bottom-right)
// - Anti-diagonal (top-right to bottom-left)
func ValidateBingo(card *BingoCard, markedNumbers []int) bool {
	if len(markedNumbers) < 5 {
		return false
	}

	// Convert marked numbers to a set for quick lookup
	marked := make(map[int]bool)
	for _, num := range markedNumbers {
		marked[num] = true
	}

	// Check rows
	for row := 0; row < 5; row++ {
		count := 0
		for col := 0; col < 5; col++ {
			num := card.Numbers[row][col]
			// Center cell is always considered marked
			if num == 0 || marked[num] {
				count++
			}
		}
		if count == 5 {
			return true
		}
	}

	// Check columns
	for col := 0; col < 5; col++ {
		count := 0
		for row := 0; row < 5; row++ {
			num := card.Numbers[row][col]
			// Center cell is always considered marked
			if num == 0 || marked[num] {
				count++
			}
		}
		if count == 5 {
			return true
		}
	}

	// Check main diagonal (top-left to bottom-right)
	count := 0
	for i := 0; i < 5; i++ {
		num := card.Numbers[i][i]
		if num == 0 || marked[num] {
			count++
		}
	}
	if count == 5 {
		return true
	}

	// Check anti-diagonal (top-right to bottom-left)
	count = 0
	for i := 0; i < 5; i++ {
		num := card.Numbers[i][4-i]
		if num == 0 || marked[num] {
			count++
		}
	}
	if count == 5 {
		return true
	}

	return false
}

// GetLetterForNumber returns the BINGO letter for a given number
func GetLetterForNumber(num int) string {
	switch {
	case num >= 1 && num <= 15:
		return "B"
	case num >= 16 && num <= 30:
		return "I"
	case num >= 31 && num <= 45:
		return "N"
	case num >= 46 && num <= 60:
		return "G"
	case num >= 61 && num <= 75:
		return "O"
	default:
		return ""
	}
}

