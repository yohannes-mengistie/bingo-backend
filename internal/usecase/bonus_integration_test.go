//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Test games in this harness cost 10 per card.
const testBet = 10.0

// grantBonus inserts a grant directly so the test controls the deadline.
// expiresInDays may be negative to model an already-expired grant.
func (h *harness) grantBonus(userID uuid.UUID, amount float64, expiresInDays int) {
	h.t.Helper()
	_, err := h.db.Exec(`
		INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
		VALUES ($1, $2, $3, $3, 'test', CURRENT_TIMESTAMP + ($4 || ' days')::interval)
	`, uuid.New(), userID, amount, expiresInDays)
	if err != nil {
		h.t.Fatalf("grant bonus: %v", err)
	}
}

// bonusBalance mirrors the repository's definition of spendable bonus.
func (h *harness) bonusBalance(userID uuid.UUID) float64 {
	h.t.Helper()
	var b float64
	if err := h.db.QueryRow(`
		SELECT COALESCE(SUM(remaining), 0)::float8 FROM bonus_grants
		WHERE user_id = $1 AND remaining > 0 AND expires_at > CURRENT_TIMESTAMP
	`, userID).Scan(&b); err != nil {
		h.t.Fatalf("read bonus balance: %v", err)
	}
	return b
}

func (h *harness) bonusFundedCards(gameID, userID uuid.UUID) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRow(`
		SELECT COUNT(*) FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND paid_from_bonus = true
	`, gameID, userID).Scan(&n); err != nil {
		h.t.Fatalf("count bonus cards: %v", err)
	}
	return n
}

// Bonus is spent before cash, so a player's own money is preserved.
func TestIntegration_Bonus_SpentBeforeCash(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusFirst", 9101)
	h.setBalance(user, 50)
	h.grantBonus(user, 2*testBet, 7) // enough for both cards
	other := h.seedUser("BonusFirstOther", 9102)
	h.setBalance(other, 50)

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 11, 0)
	h.addReservation(gameID, user, 12, 1)
	h.addReservation(gameID, other, 13, 2)
	h.forceCountdown(gameID, 2, 24)

	h.uc.startDrawing(ctx, gameID)

	if got := h.balance(user); got != 50 {
		t.Fatalf("cash balance = %.2f, want 50 — bonus should have paid for both cards", got)
	}
	if got := h.bonusBalance(user); got != 0 {
		t.Fatalf("bonus balance = %.2f, want 0 — both cards should have consumed it", got)
	}
	if got := h.bonusFundedCards(gameID, user); got != 2 {
		t.Fatalf("%d cards marked bonus-funded, want 2", got)
	}
	// The player without bonus pays cash as before.
	if got := h.balance(other); got != 40 {
		t.Fatalf("cash-only player balance = %.2f, want 40", got)
	}
}

// Bonus covering only part of a purchase pays for whole cards; cash covers the
// rest. A remainder too small for a card stays put rather than being stranded.
func TestIntegration_Bonus_PartialCoverageIsWholeCards(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusPartial", 9103)
	h.setBalance(user, 50)
	h.grantBonus(user, 15, 7) // one card's worth, with 5 left over
	other := h.seedUser("BonusPartialOther", 9104)
	h.setBalance(other, 50)

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 21, 0)
	h.addReservation(gameID, user, 22, 1)
	h.addReservation(gameID, other, 23, 2)
	h.forceCountdown(gameID, 2, 24)

	h.uc.startDrawing(ctx, gameID)

	if got := h.balance(user); got != 40 {
		t.Fatalf("cash balance = %.2f, want 40 — exactly one card should be cash-funded", got)
	}
	if got := h.bonusBalance(user); got != 5 {
		t.Fatalf("bonus balance = %.2f, want 5 — the sub-card remainder must survive", got)
	}
	if got := h.bonusFundedCards(gameID, user); got != 1 {
		t.Fatalf("%d cards marked bonus-funded, want 1", got)
	}
}

// An expired grant is invisible: it neither pays nor blocks.
func TestIntegration_Bonus_ExpiredGrantIsNotSpendable(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusExpired", 9105)
	h.setBalance(user, 50)
	h.grantBonus(user, 100, -1) // expired yesterday
	other := h.seedUser("BonusExpiredOther", 9106)
	h.setBalance(other, 50)

	if got := h.bonusBalance(user); got != 0 {
		t.Fatalf("expired grant still counts as %.2f spendable", got)
	}

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 31, 0)
	h.addReservation(gameID, other, 32, 1)
	h.forceCountdown(gameID, 2, 16)

	h.uc.startDrawing(ctx, gameID)

	if got := h.balance(user); got != 40 {
		t.Fatalf("cash balance = %.2f, want 40 — expired bonus must not pay", got)
	}
	if got := h.bonusFundedCards(gameID, user); got != 0 {
		t.Fatalf("%d cards marked bonus-funded, want 0", got)
	}
}

// Bonus smaller than one card cannot be used at all — a card is indivisible.
func TestIntegration_Bonus_BelowOneCardIsUnused(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusTiny", 9107)
	h.setBalance(user, 50)
	h.grantBonus(user, testBet-1, 7) // just under one card
	other := h.seedUser("BonusTinyOther", 9108)
	h.setBalance(other, 50)

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 41, 0)
	h.addReservation(gameID, other, 42, 1)
	h.forceCountdown(gameID, 2, 16)

	h.uc.startDrawing(ctx, gameID)

	if got := h.balance(user); got != 40 {
		t.Fatalf("cash balance = %.2f, want 40 — cash must cover the card", got)
	}
	if got := h.bonusBalance(user); got != testBet-1 {
		t.Fatalf("bonus balance = %.2f, want %.2f — under-a-card bonus must be untouched", got, testBet-1)
	}
}

// THE LEAK THIS DESIGN EXISTS TO PREVENT. A bonus-funded card that is refunded
// must come back as bonus. If it landed in the cash balance, a player could
// join a game and leave to convert play-only money into withdrawable cash.
func TestIntegration_Bonus_RefundReturnsToBonusNotCash(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusRefund", 9109)
	h.setBalance(user, 50)
	h.grantBonus(user, testBet, 7)
	other := h.seedUser("BonusRefundOther", 9110)
	h.setBalance(other, 50)

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 51, 0)
	h.addReservation(gameID, other, 52, 1)
	h.forceCountdown(gameID, 2, 16)

	h.uc.startDrawing(ctx, gameID)

	// Precondition: the card was bought with bonus, cash untouched.
	if got := h.balance(user); got != 50 {
		t.Fatalf("setup: cash = %.2f, want 50 (bonus should have paid)", got)
	}
	if got := h.bonusBalance(user); got != 0 {
		t.Fatalf("setup: bonus = %.2f, want 0 (consumed)", got)
	}

	if _, _, _, err := h.uc.cancelGameAndRefund(ctx, gameID, "test cancel"); err != nil {
		t.Fatalf("cancel game: %v", err)
	}

	if got := h.balance(user); got != 50 {
		t.Fatalf("cash balance = %.2f, want 50 — a bonus-funded stake must NOT refund as cash", got)
	}
	if got := h.bonusBalance(user); got != testBet {
		t.Fatalf("bonus balance = %.2f, want %.2f — the stake must return as bonus", got, testBet)
	}
	// The cash-funded player is refunded in cash, exactly as before.
	if got := h.balance(other); got != 50 {
		t.Fatalf("cash player balance = %.2f, want 50 after refund", got)
	}
}

// A refund reinstates bonus under its ORIGINAL deadline, so joining and leaving
// repeatedly cannot keep an expiring bonus alive forever.
func TestIntegration_Bonus_RefundKeepsOriginalDeadline(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusDeadline", 9111)
	h.setBalance(user, 50)
	h.grantBonus(user, testBet, 7)
	other := h.seedUser("BonusDeadlineOther", 9112)
	h.setBalance(other, 50)

	var originalExpiry string
	if err := h.db.QueryRow(
		`SELECT expires_at::text FROM bonus_grants WHERE user_id=$1`, user,
	).Scan(&originalExpiry); err != nil {
		t.Fatalf("read original expiry: %v", err)
	}

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 61, 0)
	h.addReservation(gameID, other, 62, 1)
	h.forceCountdown(gameID, 2, 16)
	h.uc.startDrawing(ctx, gameID)

	if _, _, _, err := h.uc.cancelGameAndRefund(ctx, gameID, "test cancel"); err != nil {
		t.Fatalf("cancel game: %v", err)
	}

	// The restored grant must carry the original deadline, not a fresh one.
	var restoredExpiry string
	if err := h.db.QueryRow(
		`SELECT expires_at::text FROM bonus_grants WHERE user_id=$1 AND remaining > 0`, user,
	).Scan(&restoredExpiry); err != nil {
		t.Fatalf("read restored expiry: %v", err)
	}
	if restoredExpiry != originalExpiry {
		t.Fatalf("refund deadline = %s, want the original %s — a round trip must not extend bonus life",
			restoredExpiry, originalExpiry)
	}
}

// Bonus must never reach the cash balance, which is the only thing withdrawal
// reads. This is the invariant that makes bonus non-withdrawable by design
// rather than by a check someone could forget.
func TestIntegration_Bonus_NeverTouchesCashBalance(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusNoCash", 9113)
	h.setBalance(user, 0)
	h.grantBonus(user, 500, 7)

	if got := h.balance(user); got != 0 {
		t.Fatalf("granting bonus moved the cash balance to %.2f — bonus must be a separate purse", got)
	}

	// Play a card entirely on bonus; cash must still be zero afterwards.
	other := h.seedUser("BonusNoCashOther", 9114)
	h.setBalance(other, 50)
	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 71, 0)
	h.addReservation(gameID, other, 72, 1)
	h.forceCountdown(gameID, 2, 16)
	h.uc.startDrawing(ctx, gameID)

	if got := h.balance(user); got != 0 {
		t.Fatalf("cash balance = %.2f after a bonus-funded stake, want 0", got)
	}
	if got := h.bonusBalance(user); got != 500-testBet {
		t.Fatalf("bonus balance = %.2f, want %.2f", got, 500-testBet)
	}
}

// A player who cannot cover the cash remainder keeps their bonus: it must not
// be consumed for cards they never received.
func TestIntegration_Bonus_NotConsumedWhenCashShort(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("BonusShort", 9115)
	h.setBalance(user, 0)          // no cash at all
	h.grantBonus(user, testBet, 7) // covers exactly one of two cards
	other := h.seedUser("BonusShortOther", 9116)
	h.setBalance(other, 50)

	gameID := h.seedWaitingGame()
	h.addReservation(gameID, user, 81, 0)
	h.addReservation(gameID, user, 82, 1) // second card has no funding
	h.addReservation(gameID, other, 83, 2)
	h.forceCountdown(gameID, 2, 24)

	h.uc.startDrawing(ctx, gameID)

	// Both of this player's cards are released, and the bonus is untouched —
	// spending it on a dropped card would destroy it for nothing.
	if got := h.bonusBalance(user); got != testBet {
		t.Fatalf("bonus balance = %.2f, want %.2f — bonus must survive a failed purchase", got, testBet)
	}
	if got := h.activeCardCount(gameID, user); got != 0 {
		t.Fatalf("%d cards still active, want 0 — unfunded cards must be released", got)
	}
}
