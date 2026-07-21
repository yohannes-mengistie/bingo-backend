//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
)

func (h *harness) userUC() *UserUseCase {
	h.t.Helper()
	return NewUserUseCase(
		postgres.NewUserRepository(h.db),
		postgres.NewWalletRepository(h.db),
		postgres.NewBonusRepository(h.db),
		h.db,
	)
}

// The reward is granted the moment the invited user signs up — as PLAY-ONLY
// bonus, NOT withdrawable cash. So the referrer's real (withdrawable) balance is
// unchanged, but their bonus balance goes up by ReferralRewardAmount.
func TestIntegration_Referral_PaidAtSignup(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.userUC()

	referrer, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000001, FirstName: "Referrer", Phone: "0911990001",
	})
	if err != nil {
		t.Fatalf("create referrer: %v", err)
	}
	h.ids.users = append(h.ids.users, referrer.ID)

	invited, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000002, FirstName: "Invited", Phone: "0911990002",
		ReferrerCode: referrer.ReferalCode,
	})
	if err != nil {
		t.Fatalf("create invited: %v", err)
	}
	h.ids.users = append(h.ids.users, invited.ID)

	// Real (withdrawable) balance stays 0 — neither the welcome credit nor the
	// referral reward is cash; both are play-only bonus.
	if got := h.balance(referrer.ID); got != 0 {
		t.Fatalf("referrer real balance = %.2f, want 0 (rewards must be bonus, not cash)", got)
	}
	// Referrer's bonus = welcome credit + the referral reward.
	if got := h.bonusBalance(referrer.ID); got != domain.DefaultUserBalance+domain.ReferralRewardAmount {
		t.Fatalf("referrer bonus balance = %.2f, want %.2f", got, domain.DefaultUserBalance+domain.ReferralRewardAmount)
	}
	// Invited user gets only the welcome bonus (not self-rewarded), 0 cash.
	if got := h.balance(invited.ID); got != 0 {
		t.Fatalf("invited balance = %.2f, want 0", got)
	}
	if got := h.bonusBalance(invited.ID); got != domain.DefaultUserBalance {
		t.Fatalf("invited bonus balance = %.2f, want %.2f (welcome only)", got, domain.DefaultUserBalance)
	}
	// Link recorded + flagged rewarded so it can never pay twice.
	if invited.ReferredBy == nil || *invited.ReferredBy != referrer.ID {
		t.Fatalf("invited.referred_by not set to referrer")
	}
	var rewarded bool
	if err := h.db.QueryRow(`SELECT referral_rewarded FROM users WHERE id=$1`, invited.ID).Scan(&rewarded); err != nil {
		t.Fatalf("read referral_rewarded: %v", err)
	}
	if !rewarded {
		t.Fatalf("invited.referral_rewarded = false, want true")
	}
}

// No code → no referrer, no reward, no error.
func TestIntegration_Referral_NoCode_NoReward(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.userUC()

	solo, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000003, FirstName: "Solo", Phone: "0911990003",
	})
	if err != nil {
		t.Fatalf("create solo: %v", err)
	}
	h.ids.users = append(h.ids.users, solo.ID)

	if solo.ReferredBy != nil {
		t.Fatalf("solo should have no referrer")
	}
	// Welcome credit is play-only bonus now: 0 cash, DefaultUserBalance bonus.
	if got := h.balance(solo.ID); got != 0 {
		t.Fatalf("solo real balance = %.2f, want 0", got)
	}
	if got := h.bonusBalance(solo.ID); got != domain.DefaultUserBalance {
		t.Fatalf("solo bonus balance = %.2f, want %.2f (welcome)", got, domain.DefaultUserBalance)
	}
}
