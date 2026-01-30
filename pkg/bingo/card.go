package bingo

import "github.com/bingo/backend/internal/domain"

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
// Card ID must be between MinCardID and MaxCardID
// Each card ID always returns the same card (deterministic)
func GenerateCard(cardID int) *BingoCard {
	if cardID < domain.MinCardID || cardID > domain.MaxCardID {
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

// GenerateAllCards returns all fixed bingo cards
func GenerateAllCards() []*BingoCard {
	cards := make([]*BingoCard, domain.TotalCards)
	for i := domain.MinCardID; i <= domain.MaxCardID; i++ {
		cards[i-domain.MinCardID] = GenerateCard(i)
	}
	return cards
}

// generateFixedCards creates all fixed bingo cards from hardcoded data
// Each card has unique numbers but the same card ID always has the same numbers
func generateFixedCards() map[int]*BingoCard {
	cards := make(map[int]*BingoCard, domain.TotalCards)

	// Hardcoded card data - each card has 5 columns: B, I, N, G, O
	cardData := [100][5][5]int{
		// Card 1
		{{15, 28, 31, 58, 61}, {5, 30, 45, 46, 73}, {3, 16, 0, 48, 65}, {1, 20, 33, 50, 75}, {12, 18, 43, 60, 63}},
		// Card 2
		{{4, 29, 44, 47, 72}, {14, 27, 42, 57, 62}, {11, 21, 0, 51, 66}, {2, 19, 32, 59, 74}, {6, 17, 34, 49, 64}},
		// Card 3
		{{1, 30, 41, 52, 67}, {13, 26, 31, 46, 75}, {7, 20, 0, 56, 71}, {5, 22, 45, 50, 61}, {10, 16, 35, 60, 65}},
		// Card 4
		{{8, 25, 33, 48, 70}, {9, 18, 40, 59, 74}, {15, 29, 0, 53, 66}, {3, 23, 44, 55, 63}, {6, 21, 36, 51, 68}},
		// Card 5
		{{14, 28, 45, 52, 73}, {7, 27, 37, 54, 67}, {11, 17, 0, 57, 62}, {9, 24, 32, 47, 72}, {2, 22, 39, 58, 69}},
		// Card 6
		{{5, 30, 44, 50, 65}, {12, 19, 34, 60, 71}, {4, 20, 0, 55, 75}, {10, 26, 38, 56, 70}, {13, 25, 35, 49, 64}},
		// Card 7
		{{6, 16, 36, 51, 74}, {1, 29, 37, 56, 75}, {11, 30, 0, 46, 61}, {14, 26, 31, 60, 66}, {15, 21, 45, 59, 71}},
		// Card 8
		{{7, 17, 32, 58, 72}, {2, 22, 41, 52, 62}, {12, 28, 0, 48, 67}, {3, 18, 33, 57, 73}, {13, 27, 42, 47, 63}},
		// Card 9
		{{8, 23, 43, 55, 70}, {15, 19, 38, 53, 75}, {9, 24, 0, 60, 68}, {10, 30, 34, 49, 64}, {4, 25, 39, 54, 69}},
		// Card 10
		{{1, 21, 36, 51, 66}, {6, 20, 44, 57, 71}, {12, 27, 0, 50, 61}, {5, 16, 35, 46, 65}, {11, 26, 31, 56, 72}},
		// Card 11
		{{2, 22, 32, 58, 73}, {13, 23, 43, 53, 68}, {8, 28, 0, 52, 67}, {14, 29, 38, 47, 74}, {7, 17, 37, 59, 62}},
		// Card 12
		{{9, 19, 39, 55, 63}, {10, 18, 33, 54, 70}, {15, 30, 0, 49, 75}, {4, 24, 40, 60, 69}, {3, 25, 34, 48, 64}},
		// Card 13
		{{7, 27, 41, 58, 67}, {13, 28, 37, 52, 73}, {12, 22, 0, 51, 61}, {1, 16, 36, 57, 66}, {6, 21, 31, 46, 72}},
		// Card 14
		{{8, 23, 35, 47, 68}, {2, 20, 38, 53, 74}, {14, 26, 0, 59, 62}, {5, 29, 32, 56, 65}, {11, 17, 42, 50, 71}},
		// Card 15
		{{9, 24, 34, 48, 63}, {10, 25, 33, 55, 64}, {4, 30, 0, 49, 70}, {15, 19, 40, 60, 69}, {3, 18, 39, 54, 75}},
		// Card 16
		{{3, 17, 45, 46, 65}, {5, 16, 44, 49, 61}, {2, 20, 0, 47, 64}, {4, 18, 32, 50, 62}, {1, 19, 31, 48, 63}},
		// Card 17
		{{10, 24, 34, 55, 66}, {7, 21, 33, 53, 67}, {9, 25, 0, 51, 69}, {8, 23, 36, 52, 70}, {6, 22, 35, 54, 68}},
		// Card 18
		{{14, 28, 39, 56, 72}, {15, 27, 40, 58, 75}, {13, 26, 0, 59, 73}, {12, 30, 37, 60, 71}, {11, 29, 38, 57, 74}},
		// Card 19
		{{15, 21, 40, 47, 62}, {6, 26, 31, 46, 75}, {11, 16, 0, 56, 71}, {2, 17, 32, 60, 66}, {1, 30, 36, 51, 61}},
		// Card 20
		{{14, 22, 37, 48, 74}, {12, 27, 33, 52, 63}, {7, 29, 0, 57, 72}, {3, 19, 39, 59, 67}, {4, 18, 34, 49, 64}},
		// Card 21
		{{8, 25, 42, 55, 65}, {9, 28, 41, 58, 69}, {5, 23, 0, 53, 70}, {10, 20, 38, 54, 68}, {13, 24, 35, 50, 73}},
		// Card 22
		{{12, 19, 31, 46, 75}, {6, 21, 36, 60, 66}, {4, 16, 0, 57, 72}, {1, 27, 45, 51, 61}, {15, 30, 34, 49, 64}},
		// Card 23
		{{2, 26, 44, 48, 63}, {11, 17, 32, 56, 62}, {3, 29, 0, 47, 74}, {7, 18, 33, 59, 67}, {14, 22, 37, 52, 71}},
		// Card 24
		{{10, 28, 35, 50, 65}, {8, 24, 38, 58, 69}, {5, 20, 0, 55, 73}, {13, 25, 43, 54, 68}, {9, 23, 41, 53, 70}},
		// Card 25
		{{14, 18, 31, 56, 74}, {11, 29, 33, 46, 71}, {1, 21, 0, 51, 66}, {3, 16, 44, 48, 61}, {6, 26, 36, 59, 63}},
		// Card 26
		{{15, 22, 37, 49, 72}, {12, 30, 32, 57, 75}, {4, 27, 0, 47, 67}, {7, 17, 45, 52, 64}, {2, 19, 34, 60, 62}},
		// Card 27
		{{5, 24, 41, 58, 70}, {8, 25, 35, 55, 69}, {9, 23, 0, 54, 73}, {10, 20, 42, 53, 65}, {13, 28, 38, 50, 68}},
		// Card 28
		{{12, 16, 33, 56, 71}, {2, 18, 43, 48, 61}, {1, 27, 0, 57, 63}, {3, 17, 32, 46, 72}, {11, 26, 31, 47, 62}},
		// Card 29
		{{5, 21, 36, 51, 66}, {6, 19, 34, 59, 73}, {4, 28, 0, 58, 64}, {14, 29, 44, 49, 65}, {13, 20, 35, 50, 74}},
		// Card 30
		{{10, 23, 45, 55, 75}, {9, 30, 39, 53, 70}, {8, 24, 0, 60, 67}, {7, 25, 38, 52, 68}, {15, 22, 37, 54, 69}},
		// Card 31
		{{1, 19, 31, 59, 67}, {7, 29, 37, 52, 71}, {11, 26, 0, 49, 64}, {14, 16, 34, 46, 74}, {4, 22, 44, 56, 61}},
		// Card 32
		{{5, 20, 38, 60, 62}, {12, 27, 45, 53, 72}, {8, 17, 0, 50, 68}, {15, 30, 35, 47, 75}, {2, 23, 32, 57, 65}},
		// Card 33
		{{10, 18, 33, 48, 66}, {13, 21, 41, 51, 63}, {9, 28, 0, 58, 70}, {6, 24, 36, 55, 73}, {3, 25, 39, 54, 69}},
		// Card 34
		{{6, 20, 34, 49, 62}, {2, 21, 31, 50, 61}, {4, 17, 0, 51, 65}, {1, 19, 32, 46, 64}, {5, 16, 35, 47, 66}},
		// Card 35
		{{11, 24, 40, 56, 68}, {7, 22, 37, 53, 70}, {8, 23, 0, 55, 69}, {10, 26, 39, 52, 71}, {9, 25, 38, 54, 67}},
		// Card 36
		{{15, 29, 39, 56, 71}, {14, 27, 40, 58, 73}, {3, 30, 0, 48, 72}, {13, 18, 41, 59, 74}, {12, 28, 42, 60, 75}},
		// Card 37
		{{13, 24, 35, 60, 61}, {5, 28, 45, 58, 69}, {15, 20, 0, 46, 75}, {9, 30, 31, 50, 65}, {1, 16, 39, 54, 73}},
		// Card 38
		{{2, 26, 36, 55, 74}, {11, 29, 32, 59, 71}, {14, 25, 0, 51, 70}, {6, 17, 41, 56, 66}, {10, 21, 40, 47, 62}},
		// Card 39
		{{3, 22, 38, 48, 72}, {4, 19, 33, 52, 64}, {12, 27, 0, 53, 68}, {8, 23, 37, 57, 63}, {7, 18, 34, 49, 67}},
		// Card 40
		{{9, 22, 33, 46, 65}, {1, 16, 35, 48, 67}, {5, 20, 0, 54, 69}, {3, 18, 37, 52, 61}, {7, 24, 31, 50, 63}},
		// Card 41
		{{6, 21, 38, 53, 62}, {2, 25, 32, 49, 64}, {8, 19, 0, 55, 68}, {10, 23, 36, 47, 66}, {4, 17, 34, 51, 70}},
		// Card 42
		{{14, 30, 39, 56, 74}, {15, 26, 41, 59, 71}, {12, 27, 0, 58, 72}, {11, 29, 42, 60, 73}, {13, 28, 45, 57, 75}},
		// Card 43
		{{2, 23, 32, 46, 62}, {8, 30, 38, 47, 61}, {1, 17, 0, 60, 68}, {15, 24, 31, 54, 75}, {9, 16, 45, 53, 69}},
		// Card 44
		{{10, 26, 40, 48, 64}, {4, 25, 33, 49, 63}, {5, 20, 0, 50, 70}, {11, 19, 35, 55, 65}, {3, 18, 34, 56, 71}},
		// Card 45
		{{14, 28, 37, 52, 73}, {7, 27, 41, 57, 72}, {12, 22, 0, 51, 74}, {13, 21, 42, 59, 67}, {6, 29, 36, 58, 66}},
		// Card 46
		{{6, 16, 36, 54, 66}, {12, 27, 31, 57, 72}, {4, 19, 0, 49, 64}, {9, 24, 34, 46, 69}, {1, 21, 39, 51, 61}},
		// Card 47
		{{13, 25, 32, 55, 62}, {2, 22, 37, 47, 70}, {5, 17, 0, 52, 73}, {10, 28, 40, 50, 65}, {7, 20, 35, 58, 67}},
		// Card 48
		{{14, 23, 44, 48, 74}, {11, 29, 41, 53, 68}, {8, 18, 0, 60, 63}, {15, 30, 33, 59, 75}, {3, 26, 38, 56, 71}},
		// Card 49
		{{3, 18, 31, 60, 75}, {7, 26, 45, 46, 61}, {15, 30, 0, 48, 63}, {1, 16, 37, 56, 67}, {11, 22, 33, 52, 71}},
		// Card 50
		{{2, 29, 32, 53, 62}, {8, 27, 44, 47, 68}, {14, 17, 0, 57, 72}, {4, 19, 38, 49, 64}, {12, 23, 34, 59, 74}},
		// Card 51
		{{6, 25, 35, 50, 69}, {10, 28, 41, 58, 70}, {13, 24, 0, 55, 66}, {5, 20, 36, 54, 65}, {9, 21, 39, 51, 73}},
		// Card 52
		{{8, 24, 35, 46, 62}, {5, 23, 32, 50, 69}, {9, 17, 0, 54, 65}, {2, 16, 31, 47, 68}, {1, 20, 38, 53, 61}},
		// Card 53
		{{3, 19, 37, 52, 70}, {11, 22, 40, 48, 67}, {7, 25, 0, 55, 71}, {10, 26, 33, 49, 64}, {4, 18, 34, 56, 63}},
		// Card 54
		{{12, 30, 43, 57, 66}, {15, 21, 36, 58, 75}, {13, 27, 0, 51, 73}, {6, 28, 42, 60, 72}, {14, 29, 41, 59, 74}},
		// Card 55
		{{3, 25, 33, 46, 63}, {6, 18, 36, 51, 70}, {14, 16, 0, 55, 66}, {1, 29, 40, 59, 74}, {10, 21, 31, 48, 61}},
		// Card 56
		{{2, 17, 39, 58, 73}, {4, 28, 32, 54, 67}, {13, 22, 0, 47, 62}, {7, 24, 34, 49, 64}, {9, 19, 37, 52, 69}},
		// Card 57
		{{5, 20, 38, 50, 68}, {11, 26, 41, 56, 72}, {8, 30, 0, 60, 75}, {15, 23, 45, 57, 65}, {12, 27, 35, 53, 71}},
		// Card 58
		{{1, 22, 35, 46, 64}, {4, 19, 34, 53, 61}, {8, 23, 0, 50, 67}, {7, 16, 31, 49, 68}, {5, 20, 37, 52, 65}},
		// Card 59
		{{2, 24, 39, 51, 66}, {3, 25, 32, 54, 63}, {9, 18, 0, 48, 70}, {6, 21, 33, 55, 69}, {10, 17, 36, 47, 62}},
		// Card 60
		{{12, 30, 42, 59, 71}, {13, 27, 43, 58, 72}, {11, 29, 0, 56, 74}, {14, 28, 41, 57, 73}, {15, 26, 40, 60, 75}},
		// Card 61
		{{4, 19, 44, 46, 65}, {3, 20, 32, 48, 63}, {5, 18, 0, 49, 62}, {2, 17, 31, 50, 64}, {1, 16, 45, 47, 61}},
		// Card 62
		{{10, 23, 36, 51, 66}, {9, 22, 34, 52, 68}, {8, 21, 0, 54, 70}, {7, 25, 33, 53, 69}, {6, 24, 35, 55, 67}},
		// Card 63
		{{11, 29, 39, 58, 71}, {14, 28, 40, 56, 73}, {15, 30, 0, 59, 72}, {12, 27, 38, 60, 74}, {13, 26, 37, 57, 75}},
		// Card 64
		{{3, 22, 35, 52, 67}, {1, 16, 31, 46, 69}, {7, 18, 0, 48, 61}, {9, 20, 33, 50, 65}, {5, 24, 37, 54, 63}},
		// Card 65
		{{6, 23, 38, 47, 62}, {2, 25, 36, 53, 64}, {8, 19, 0, 55, 70}, {4, 21, 34, 49, 68}, {10, 17, 32, 51, 66}},
		// Card 66
		{{12, 27, 41, 58, 73}, {13, 30, 45, 57, 75}, {14, 26, 0, 60, 74}, {15, 29, 42, 56, 72}, {11, 28, 39, 59, 71}},
		// Card 67
		{{15, 26, 32, 56, 66}, {1, 21, 36, 51, 71}, {11, 17, 0, 60, 61}, {6, 30, 40, 47, 75}, {2, 16, 31, 46, 62}},
		// Card 68
		{{14, 22, 37, 48, 72}, {12, 27, 33, 52, 67}, {7, 29, 0, 57, 63}, {4, 18, 39, 49, 64}, {3, 19, 34, 59, 74}},
		// Card 69
		{{10, 24, 35, 54, 73}, {8, 25, 42, 58, 65}, {5, 20, 0, 53, 68}, {13, 23, 38, 55, 70}, {9, 28, 41, 50, 69}},
		// Card 70
		{{1, 27, 36, 57, 72}, {6, 30, 31, 60, 75}, {15, 19, 0, 46, 66}, {4, 16, 45, 49, 64}, {12, 21, 34, 51, 61}},
		// Card 71
		{{14, 26, 37, 56, 74}, {11, 17, 44, 47, 62}, {7, 18, 0, 59, 67}, {3, 22, 32, 52, 63}, {2, 29, 33, 48, 71}},
		// Card 72
		{{9, 25, 38, 55, 69}, {8, 24, 43, 50, 65}, {10, 20, 0, 58, 73}, {5, 23, 41, 53, 70}, {13, 28, 35, 54, 68}},
		// Card 73
		{{14, 29, 44, 51, 66}, {3, 18, 31, 48, 74}, {11, 16, 0, 59, 61}, {1, 21, 36, 46, 71}, {6, 26, 33, 56, 63}},
		// Card 74
		{{4, 27, 37, 49, 75}, {7, 30, 45, 52, 67}, {15, 17, 0, 47, 64}, {2, 19, 34, 60, 62}, {12, 22, 32, 57, 72}},
		// Card 75
		{{13, 28, 35, 58, 70}, {5, 20, 38, 53, 65}, {9, 24, 0, 50, 73}, {10, 23, 42, 55, 69}, {8, 25, 41, 54, 68}},
		// Card 76
		{{12, 16, 43, 56, 72}, {1, 26, 33, 46, 71}, {3, 17, 0, 48, 62}, {11, 18, 31, 47, 63}, {2, 27, 32, 57, 61}},
		// Card 77
		{{6, 20, 36, 58, 73}, {5, 28, 44, 49, 66}, {4, 21, 0, 50, 65}, {13, 29, 34, 51, 74}, {14, 19, 35, 59, 64}},
		// Card 78
		{{10, 22, 37, 60, 75}, {9, 30, 45, 52, 70}, {8, 25, 0, 55, 69}, {15, 24, 38, 54, 68}, {7, 23, 39, 53, 67}},
		// Card 79
		{{7, 26, 34, 49, 67}, {14, 29, 44, 46, 74}, {4, 22, 0, 52, 64}, {1, 16, 31, 56, 61}, {11, 19, 37, 59, 71}},
		// Card 80
		{{15, 17, 35, 60, 62}, {8, 27, 45, 47, 68}, {2, 23, 0, 50, 75}, {12, 30, 32, 57, 65}, {5, 20, 38, 53, 72}},
		// Card 81
		{{13, 25, 36, 55, 66}, {10, 18, 41, 58, 69}, {3, 21, 0, 48, 73}, {6, 28, 33, 51, 63}, {9, 24, 39, 54, 70}},
		// Card 82
		{{1, 20, 35, 49, 66}, {2, 19, 34, 51, 61}, {4, 21, 0, 46, 62}, {6, 17, 31, 50, 65}, {5, 16, 32, 47, 64}},
		// Card 83
		{{10, 24, 40, 53, 70}, {9, 23, 39, 52, 67}, {8, 25, 0, 54, 69}, {11, 26, 38, 55, 71}, {7, 22, 37, 56, 68}},
		// Card 84
		{{13, 30, 44, 60, 75}, {15, 18, 43, 48, 63}, {3, 27, 0, 57, 72}, {14, 28, 41, 59, 74}, {12, 29, 42, 58, 73}},
		// Card 85
		{{9, 30, 35, 54, 73}, {13, 16, 39, 60, 61}, {15, 20, 0, 46, 65}, {5, 24, 31, 58, 75}, {1, 28, 45, 50, 69}},
		// Card 86
		{{2, 17, 36, 55, 71}, {10, 26, 32, 47, 66}, {11, 21, 0, 56, 74}, {14, 25, 41, 51, 62}, {6, 29, 40, 59, 70}},
		// Card 87
		{{8, 19, 33, 53, 63}, {12, 23, 37, 49, 72}, {7, 22, 0, 48, 64}, {3, 27, 38, 52, 67}, {4, 18, 34, 57, 68}},
		// Card 88
		{{7, 22, 35, 52, 61}, {9, 16, 31, 46, 63}, {1, 20, 0, 54, 67}, {5, 24, 33, 50, 65}, {3, 18, 37, 48, 69}},
		// Card 89
		{{6, 25, 38, 53, 68}, {4, 19, 36, 49, 64}, {10, 23, 0, 55, 70}, {8, 21, 34, 51, 66}, {2, 17, 32, 47, 62}},
		// Card 90
		{{14, 26, 39, 56, 71}, {13, 28, 41, 59, 74}, {12, 30, 0, 58, 72}, {15, 27, 42, 57, 73}, {11, 29, 45, 60, 75}},
		// Card 91
		{{2, 16, 32, 54, 62}, {8, 23, 38, 46, 75}, {1, 17, 0, 60, 68}, {15, 30, 31, 47, 61}, {9, 24, 45, 53, 69}},
		// Card 92
		{{11, 26, 40, 56, 70}, {3, 25, 33, 50, 63}, {4, 20, 0, 55, 64}, {10, 18, 35, 48, 65}, {5, 19, 34, 49, 71}},
		// Card 93
		{{7, 27, 36, 58, 67}, {14, 21, 37, 59, 66}, {12, 28, 0, 52, 74}, {13, 22, 42, 57, 72}, {6, 29, 41, 51, 73}},
		// Card 94
		{{4, 24, 31, 54, 72}, {1, 21, 36, 46, 66}, {12, 16, 0, 49, 69}, {9, 27, 34, 51, 64}, {6, 19, 39, 57, 61}},
		// Card 95
		{{7, 17, 40, 55, 67}, {5, 25, 35, 47, 73}, {2, 20, 0, 58, 70}, {10, 28, 32, 52, 65}, {13, 22, 37, 50, 62}},
		// Card 96
		{{8, 23, 44, 60, 63}, {11, 29, 33, 59, 75}, {14, 30, 0, 48, 74}, {15, 26, 41, 56, 71}, {3, 18, 38, 53, 68}},
		// Card 97
		{{3, 30, 37, 48, 67}, {1, 26, 45, 46, 71}, {7, 18, 0, 60, 63}, {15, 16, 31, 56, 75}, {11, 22, 33, 52, 61}},
		// Card 98
		{{2, 19, 44, 53, 62}, {4, 27, 38, 57, 72}, {14, 23, 0, 47, 74}, {12, 17, 34, 49, 64}, {8, 29, 32, 59, 68}},
		// Card 99
		{{5, 21, 41, 51, 66}, {10, 24, 39, 58, 70}, {13, 25, 0, 54, 65}, {9, 20, 36, 55, 73}, {6, 28, 35, 50, 69}},
		// Card 100
		{{9, 16, 38, 53, 65}, {2, 24, 31, 46, 69}, {8, 20, 0, 50, 61}, {5, 23, 32, 47, 62}, {1, 17, 35, 54, 68}},
	}

	// Parse card data into BingoCard structures
	for cardID := domain.MinCardID; cardID <= domain.MaxCardID; cardID++ {
		card := &BingoCard{
			ID:      cardID,
			Numbers: cardData[cardID-domain.MinCardID],
		}

		// Validate card follows bingo rules
		if !validateCard(card) {
			// Fix any violations
			fixCard(card)
		}

		cards[cardID] = card
	}

	return cards
}

// validateCard checks if a card follows bingo rules
func validateCard(card *BingoCard) bool {
	ranges := [5]struct{ min, max int }{
		{domain.BingoNumberMinB, domain.BingoNumberMaxB}, // B
		{domain.BingoNumberMinI, domain.BingoNumberMaxI}, // I
		{domain.BingoNumberMinN, domain.BingoNumberMaxN}, // N
		{domain.BingoNumberMinG, domain.BingoNumberMaxG}, // G
		{domain.BingoNumberMinO, domain.BingoNumberMaxO}, // O
	}

	// Check each column
	for col := 0; col < domain.CardGridSize; col++ {
		used := make(map[int]bool)
		for row := 0; row < domain.CardGridSize; row++ {
			num := card.Numbers[row][col]

			// Center cell should be CardCenterValue
			if col == domain.CardCenterCol && row == domain.CardCenterRow {
				if num != domain.CardCenterValue {
					return false
				}
				continue
			}

			// Check number is in valid range
			if num < ranges[col].min || num > ranges[col].max {
				return false
			}

			// Check for duplicates in column
			if used[num] {
				return false
			}
			used[num] = true
		}
	}

	return true
}

// fixCard fixes any bingo rule violations in a card
func fixCard(card *BingoCard) {
	ranges := [5]struct{ min, max int }{
		{domain.BingoNumberMinB, domain.BingoNumberMaxB}, // B
		{domain.BingoNumberMinI, domain.BingoNumberMaxI}, // I
		{domain.BingoNumberMinN, domain.BingoNumberMaxN}, // N
		{domain.BingoNumberMinG, domain.BingoNumberMaxG}, // G
		{domain.BingoNumberMinO, domain.BingoNumberMaxO}, // O
	}

	// Fix each column
	for col := 0; col < domain.CardGridSize; col++ {
		used := make(map[int]bool)
		available := make([]int, 0)

		// Build available numbers for this column
		for n := ranges[col].min; n <= ranges[col].max; n++ {
			available = append(available, n)
		}

		for row := 0; row < domain.CardGridSize; row++ {
			// Center cell should be CardCenterValue
			if col == domain.CardCenterCol && row == domain.CardCenterRow {
				card.Numbers[row][col] = domain.CardCenterValue
				continue
			}

			num := card.Numbers[row][col]

			// If number is out of range or duplicate, fix it
			if num < ranges[col].min || num > ranges[col].max || used[num] {
				// Find first available number
				for _, availNum := range available {
					if !used[availNum] {
						card.Numbers[row][col] = availNum
						used[availNum] = true
						break
					}
				}
			} else {
				used[num] = true
			}
		}
	}
}

// ValidateBingo checks if the marked numbers form a valid bingo
// A valid bingo can be:
// - Any row (5 numbers)
// - Any column (5 numbers)
// - Main diagonal (top-left to bottom-right)
// - Anti-diagonal (top-right to bottom-left)
func ValidateBingo(card *BingoCard, markedNumbers []int) bool {
	if len(markedNumbers) < 4 {
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
