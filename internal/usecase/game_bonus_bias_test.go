package usecase

import (
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/bingo"
	"github.com/google/uuid"
)

func TestIsBonusBiasTargetIncludesBonusCardForMixedPlayer(t *testing.T) {
	mixedUser := uuid.New()
	leftAt := time.Now()

	tests := []struct {
		name   string
		player *domain.GamePlayer
		want   bool
	}{
		{
			name:   "bonus card for bonus-only player",
			player: &domain.GamePlayer{UserID: uuid.New(), Paid: true, PaidFromBonus: true},
			want:   true,
		},
		{
			name:   "bonus card for mixed player",
			player: &domain.GamePlayer{UserID: mixedUser, Paid: true, PaidFromBonus: true},
			want:   true,
		},
		{
			name:   "wallet card for same mixed player",
			player: &domain.GamePlayer{UserID: mixedUser, Paid: true},
			want:   false,
		},
		{
			name:   "wallet-only player",
			player: &domain.GamePlayer{UserID: uuid.New(), Paid: true},
			want:   false,
		},
		{
			name:   "unpaid bonus reservation",
			player: &domain.GamePlayer{UserID: uuid.New(), PaidFromBonus: true},
			want:   false,
		},
		{
			name:   "bonus-funded bot",
			player: &domain.GamePlayer{UserID: uuid.New(), Paid: true, PaidFromBonus: true, IsBot: true},
			want:   false,
		},
		{
			name:   "eliminated bonus card",
			player: &domain.GamePlayer{UserID: uuid.New(), Paid: true, PaidFromBonus: true, IsEliminated: true},
			want:   false,
		},
		{
			name:   "left bonus card",
			player: &domain.GamePlayer{UserID: uuid.New(), Paid: true, PaidFromBonus: true, LeftAt: &leftAt},
			want:   false,
		},
		{
			name: "nil player",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBonusBiasTarget(tt.player); got != tt.want {
				t.Fatalf("isBonusBiasTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBonusSafeDrawCandidatesUsesFullHopperAndBlocksBonusCompletion(t *testing.T) {
	bonusCard := bingo.GenerateCard(1)
	if bonusCard == nil {
		t.Fatal("expected generated card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < 4; col++ {
		drawnSet[bonusCard.Numbers[0][col]] = true
	}
	completingNumber := bonusCard.Numbers[0][4]

	bonusPlayers := []*domain.GamePlayer{{
		UserID:        uuid.New(),
		CardID:        1,
		Paid:          true,
		PaidFromBonus: true,
	}}
	bonusCandidates := bonusSafeDrawCandidates(bonusPlayers, drawnSet)
	if containsNumber(bonusCandidates, completingNumber) {
		t.Fatalf("bonus completing number %d must be temporarily excluded", completingNumber)
	}

	walletPlayers := []*domain.GamePlayer{{
		UserID: uuid.New(),
		CardID: 1,
		Paid:   true,
	}}
	walletCandidates := bonusSafeDrawCandidates(walletPlayers, drawnSet)
	if !containsNumber(walletCandidates, completingNumber) {
		t.Fatalf("wallet completing number %d must remain in the normal hopper", completingNumber)
	}

	outsideCardNumber := 0
	cardNumbers := make(map[int]bool)
	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			cardNumbers[bonusCard.Numbers[row][col]] = true
		}
	}
	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		if !drawnSet[n] && !cardNumbers[n] {
			outsideCardNumber = n
			break
		}
	}
	if outsideCardNumber == 0 || !containsNumber(bonusCandidates, outsideCardNumber) {
		t.Fatal("candidate pool must include undrawn numbers outside tracked cards")
	}
}

func TestBonusSafeDrawCandidatesBlocksFourCornersCompletion(t *testing.T) {
	bonusCard := bingo.GenerateCard(1)
	if bonusCard == nil {
		t.Fatal("expected generated card")
	}

	drawnSet := map[int]bool{
		bonusCard.Numbers[0][0]: true,
		bonusCard.Numbers[0][4]: true,
		bonusCard.Numbers[4][0]: true,
	}
	completingCorner := bonusCard.Numbers[4][4]

	bonusPlayers := []*domain.GamePlayer{{
		UserID:        uuid.New(),
		CardID:        bonusCard.ID,
		Paid:          true,
		PaidFromBonus: true,
	}}
	if candidates := bonusSafeDrawCandidates(bonusPlayers, drawnSet); containsNumber(candidates, completingCorner) {
		t.Fatalf("four-corners completing number %d must be temporarily excluded", completingCorner)
	}

	walletPlayers := []*domain.GamePlayer{{
		UserID: uuid.New(),
		CardID: bonusCard.ID,
		Paid:   true,
	}}
	if candidates := bonusSafeDrawCandidates(walletPlayers, drawnSet); !containsNumber(candidates, completingCorner) {
		t.Fatalf("wallet four-corners number %d must remain in the normal hopper", completingCorner)
	}
}

func containsNumber(numbers []int, target int) bool {
	for _, n := range numbers {
		if n == target {
			return true
		}
	}
	return false
}
