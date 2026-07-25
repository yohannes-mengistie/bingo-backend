package usecase

import (
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
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
