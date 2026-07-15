package bingo

import (
	"testing"

	"github.com/bingo/backend/internal/domain"
)

// The generated card table must be self-consistent so the Go backend and the TS
// frontend (which mirrors cardData500) can never disagree on a card:
//   - exactly TotalCards entries,
//   - every card already valid (so fixCard is a no-op — if a fix ran, the Go
//     table would silently differ from the raw TS mirror),
//   - all cards distinct.
func TestGeneratedCardsValidAndDistinct(t *testing.T) {
	if len(cardData500) != domain.TotalCards {
		t.Fatalf("cardData500 has %d cards, want %d", len(cardData500), domain.TotalCards)
	}
	seen := make(map[[5][5]int]int, len(cardData500))
	for id := domain.MinCardID; id <= domain.MaxCardID; id++ {
		raw := cardData500[id-1]
		if !validateCard(&BingoCard{ID: id, Numbers: raw}) {
			t.Fatalf("card %d is invalid — fixCard would alter it and diverge from the TS table", id)
		}
		if got := GenerateCard(id); got == nil || got.Numbers != raw {
			t.Fatalf("card %d: GenerateCard does not match the raw table (a fix ran)", id)
		}
		if prev, dup := seen[raw]; dup {
			t.Fatalf("card %d is a duplicate of card %d", id, prev)
		}
		seen[raw] = id
	}
}
