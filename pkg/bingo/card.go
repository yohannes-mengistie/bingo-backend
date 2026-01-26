package bingo

// BingoCard represents a 5x5 bingo card
type BingoCard struct {
	ID      int       `json:"id"`
	Numbers [5][5]int `json:"numbers"` // 5x5 grid: [row][col]
}

var (
	// fixedCards stores all 100 predefined bingo cards
	// Card ID 1-100, each with fixed numbers
	fixedCards map[int]*BingoCard
)

func init() {
	// Initialize fixed cards on package load
	fixedCards = generateFixedCards()
}

// GenerateCard returns a fixed bingo card for the given card ID
// Card ID must be between 1 and 100
// Each card ID always returns the same card (deterministic)
func GenerateCard(cardID int) *BingoCard {
	if cardID < 1 || cardID > 100 {
		return nil
	}

	card, exists := fixedCards[cardID]
	if !exists {
		return nil
	}

	// Return a copy to prevent modification
	return &BingoCard{
		ID:      card.ID,
		Numbers: card.Numbers,
	}
}

// GenerateAllCards returns all 100 fixed bingo cards
func GenerateAllCards() []*BingoCard {
	cards := make([]*BingoCard, 100)
	for i := 1; i <= 100; i++ {
		cards[i-1] = GenerateCard(i)
	}
	return cards
}

// generateFixedCards creates all 100 fixed bingo cards
// Each card has unique numbers but the same card ID always has the same numbers
// Uses a deterministic algorithm to ensure consistency
func generateFixedCards() map[int]*BingoCard {
	cards := make(map[int]*BingoCard, 100)

	// Predefined number pools for each column
	// B: 1-15, I: 16-30, N: 31-45, G: 46-60, O: 61-75
	bNumbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	iNumbers := []int{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30}
	nNumbers := []int{31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45}
	gNumbers := []int{46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60}
	oNumbers := []int{61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75}

	// Generate 100 unique cards using deterministic algorithm
	for cardID := 1; cardID <= 100; cardID++ {
		card := &BingoCard{
			ID:      cardID,
			Numbers: [5][5]int{},
		}

		// Use cardID as seed for deterministic selection
		seed := int64(cardID)

		// Generate numbers for each column
		for col := 0; col < 5; col++ {
			var source []int
			switch col {
			case 0: // B
				source = bNumbers
			case 1: // I
				source = iNumbers
			case 2: // N
				source = nNumbers
			case 3: // G
				source = gNumbers
			case 4: // O
				source = oNumbers
			}

			// Select 5 unique numbers for this column
			selected := make(map[int]bool)
			rowIndex := 0

			for row := 0; row < 5; row++ {
				// Center cell (col 2, row 2) is free
				if col == 2 && row == 2 {
					card.Numbers[row][col] = 0
					continue
				}

				// Deterministic selection using cardID and position
				// This ensures same cardID always gets same numbers
				attempts := 0
				for attempts < len(source) {
					// Use a deterministic index based on cardID, column, and row
					index := int((seed*int64(col+1)*int64(row+1) + int64(attempts)) % int64(len(source)))
					num := source[index]

					if !selected[num] {
						card.Numbers[row][col] = num
						selected[num] = true
						break
					}
					attempts++
				}

				// Fallback: if all numbers are selected, pick first available
				if attempts >= len(source) {
					for _, num := range source {
						if !selected[num] {
							card.Numbers[row][col] = num
							selected[num] = true
							break
						}
					}
				}

				rowIndex++
			}
		}

		cards[cardID] = card
	}

	return cards
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
