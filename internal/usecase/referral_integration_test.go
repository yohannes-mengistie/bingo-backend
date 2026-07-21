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
		postgres.NewTransactionRepository(h.db),
		h.db,
	)
}

func (h *harness) refRewardCount(userID string) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRow(
		`SELECT count(*) FROM transactions WHERE user_id=$1 AND category='referral_reward'`, userID,
	).Scan(&n); err != nil {
		h.t.Fatalf("count referral rewards: %v", err)
	}
	return n
}

// The reward is now paid the moment the invited user signs up — no deposit
// required. The referrer's balance jumps by ReferralRewardAmount, a
// referral_reward transaction is recorded, and the link is stamped on the
// invited user.
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

	// Referrer paid immediately: started at DefaultUserBalance, now +reward.
	want := domain.DefaultUserBalance + domain.ReferralRewardAmount
	if got := h.balance(referrer.ID); got != want {
		t.Fatalf("referrer balance = %.2f, want %.2f (paid at signup)", got, want)
	}
	if n := h.refRewardCount(referrer.ID.String()); n != 1 {
		t.Fatalf("referrer referral_reward transactions = %d, want 1", n)
	}
	// Invited user is not self-credited.
	if got := h.balance(invited.ID); got != domain.DefaultUserBalance {
		t.Fatalf("invited balance = %.2f, want %.2f", got, domain.DefaultUserBalance)
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
	if got := h.balance(solo.ID); got != domain.DefaultUserBalance {
		t.Fatalf("solo balance = %.2f, want %.2f", got, domain.DefaultUserBalance)
	}
}
