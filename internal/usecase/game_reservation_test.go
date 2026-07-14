//go:build integration

// Integration tests for the reservation model: picking a card only reserves it
// (no charge); the stake is charged for everyone when the countdown ends and the
// game starts. Run against the local dev DB/Redis, same as game_integration_test:
//
//	DB_HOST=127.0.0.1 DB_USER=postgres DB_PASSWORD=... DB_NAME=bingo \
//	  go test -tags=integration -run Reservation ./internal/usecase/ -v
package usecase

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

func (h *harness) seedWaitingGame() uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	_, err := h.db.Exec(
		`INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut)
		 VALUES ($1,'REGULAR','WAITING',10,2,0,0,0.2)`, id)
	if err != nil {
		h.t.Fatalf("seed waiting game: %v", err)
	}
	h.ids.games = append(h.ids.games, id)
	return id
}

func (h *harness) setBalance(userID uuid.UUID, bal float64) {
	h.t.Helper()
	if _, err := h.db.Exec(`UPDATE wallets SET balance=$2 WHERE user_id=$1`, userID, bal); err != nil {
		h.t.Fatalf("set balance: %v", err)
	}
}

// addReservation inserts an UNPAID card row directly (bypassing JoinGame), so
// the commit path can be tested in isolation from the async countdown goroutine.
func (h *harness) addReservation(gameID, userID uuid.UUID, cardID, order int) {
	h.t.Helper()
	joinedAt := time.Now().Add(time.Duration(order) * time.Second)
	if _, err := h.db.Exec(
		`INSERT INTO game_players (id, game_id, user_id, card_id, paid, is_eliminated, joined_at)
		 VALUES ($1,$2,$3,$4,false,false,$5)`,
		uuid.New(), gameID, userID, cardID, joinedAt); err != nil {
		h.t.Fatalf("add reservation: %v", err)
	}
}

func (h *harness) forceCountdown(gameID uuid.UUID, playerCount int, prize float64) {
	h.t.Helper()
	if _, err := h.db.Exec(
		`UPDATE games SET state='COUNTDOWN', player_count=$2, prize_pool=$3,
		 countdown_ends = now() + interval '40 seconds' WHERE id=$1`,
		gameID, playerCount, prize); err != nil {
		h.t.Fatalf("force countdown: %v", err)
	}
}

// cardState returns (paid, active) for a specific card row.
func (h *harness) cardState(gameID, userID uuid.UUID, cardID int) (paid bool, active bool) {
	h.t.Helper()
	var leftAt sql.NullTime
	err := h.db.QueryRow(
		`SELECT paid, left_at FROM game_players WHERE game_id=$1 AND user_id=$2 AND card_id=$3`,
		gameID, userID, cardID).Scan(&paid, &leftAt)
	if err != nil {
		return false, false
	}
	return paid, !leftAt.Valid
}

func (h *harness) gameState(gameID uuid.UUID) *domain.Game {
	h.t.Helper()
	g, err := h.uc.gameRepo.FindByID(context.Background(), gameID)
	if err != nil {
		h.t.Fatalf("find game: %v", err)
	}
	return g
}

// Reserving a card must NOT move any money — the wallet is untouched until the
// countdown ends.
func TestIntegration_Reservation_NoChargeUntilStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("Reserver", 51)
	h.setBalance(user, 50)
	gameID := h.seedWaitingGame()

	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 1}); err != nil {
		t.Fatalf("reserve card 1: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 2}); err != nil {
		t.Fatalf("reserve card 2: %v", err)
	}

	if b := h.balance(user); b != 50 {
		t.Fatalf("balance changed on reserve: want 50, got %v", b)
	}
	if paid, active := h.cardState(gameID, user, 1); paid || !active {
		t.Fatalf("card 1 should be active+unpaid, got paid=%v active=%v", paid, active)
	}
	if paid, _ := h.cardState(gameID, user, 2); paid {
		t.Fatalf("card 2 should be unpaid")
	}
	t.Log("reserve OK: two cards reserved, wallet still 50")
}

// When the countdown ends, every reserved card is charged and the game starts.
func TestIntegration_Reservation_ChargeAtStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	u1 := h.seedUser("Alice", 52)
	u2 := h.seedUser("Bob", 53)
	h.setBalance(u1, 50)
	h.setBalance(u2, 50)
	gameID := h.seedWaitingGame()

	// Two reserved cards; force the pre-start state, then commit.
	h.addReservation(gameID, u1, 10, 0)
	h.addReservation(gameID, u2, 20, 1)
	h.forceCountdown(gameID, 2, 16) // projected pool 2*10*0.8

	h.uc.startDrawing(ctx, gameID)

	g := h.gameState(gameID)
	if g.State != domain.GameStateDrawing {
		t.Fatalf("expected DRAWING, got %s", g.State)
	}
	if b := h.balance(u1); b != 40 {
		t.Fatalf("u1 not charged: want 40, got %v", b)
	}
	if b := h.balance(u2); b != 40 {
		t.Fatalf("u2 not charged: want 40, got %v", b)
	}
	if paid, active := h.cardState(gameID, u1, 10); !paid || !active {
		t.Fatalf("u1 card should be paid+active, got paid=%v active=%v", paid, active)
	}
	if g.PrizePool != 16 {
		t.Fatalf("prize pool: want 16, got %v", g.PrizePool)
	}
	if g.PlayerCount != 2 {
		t.Fatalf("player count: want 2, got %d", g.PlayerCount)
	}
	t.Log("commit OK: both charged 10, DRAWING, pool 16")
}

// A reserver who can no longer cover their cards at commit is dropped without a
// charge; if that leaves too few players the game reverts to WAITING.
func TestIntegration_Reservation_DropUnfundedAtStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	u1 := h.seedUser("Payer", 54)
	u2 := h.seedUser("Broke", 55)
	h.setBalance(u1, 50)
	h.setBalance(u2, 50)
	gameID := h.seedWaitingGame()

	h.addReservation(gameID, u1, 11, 0)
	h.addReservation(gameID, u2, 21, 1)
	// Bob spent his money between reserving and the countdown ending.
	h.setBalance(u2, 5)
	h.forceCountdown(gameID, 2, 16)

	h.uc.startDrawing(ctx, gameID)

	g := h.gameState(gameID)
	if b := h.balance(u1); b != 40 {
		t.Fatalf("u1 should be charged 10: want 40, got %v", b)
	}
	if b := h.balance(u2); b != 5 {
		t.Fatalf("u2 should NOT be charged: want 5, got %v", b)
	}
	if _, active := h.cardState(gameID, u2, 21); active {
		t.Fatalf("u2 unfunded card should be dropped")
	}
	if paid, active := h.cardState(gameID, u1, 11); !paid || !active {
		t.Fatalf("u1 card should be paid+active, got paid=%v active=%v", paid, active)
	}
	// Only one paying player remains → cannot start.
	if g.State != domain.GameStateWaiting {
		t.Fatalf("expected revert to WAITING, got %s", g.State)
	}
	if g.PlayerCount != 1 {
		t.Fatalf("player count: want 1, got %d", g.PlayerCount)
	}
	t.Log("drop OK: unfunded card released, no charge, reverted to WAITING")
}
