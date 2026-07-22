package usecase

import (
	"testing"

	"github.com/google/uuid"
)

func TestBotBiasStartDrawCountIsStableAndVaried(t *testing.T) {
	seenMedium := false
	seenLate := false
	seenLong := false

	for i := 0; i < 256; i++ {
		var gameID uuid.UUID
		gameID[15] = byte(i)

		first := botBiasStartDrawCount(gameID)
		second := botBiasStartDrawCount(gameID)
		if first != second {
			t.Fatalf("expected stable threshold for %s, got %d then %d", gameID, first, second)
		}
		if first < botBiasMediumDrawMin || first > botBiasLongDrawMax {
			t.Fatalf("threshold out of range for %s: %d", gameID, first)
		}

		switch {
		case first <= botBiasMediumDrawMax:
			seenMedium = true
		case first <= botBiasLateDrawMax:
			seenLate = true
		default:
			seenLong = true
		}
	}

	if !seenMedium || !seenLate || !seenLong {
		t.Fatalf("expected medium, late, and long bot-bias thresholds, got medium=%v late=%v long=%v", seenMedium, seenLate, seenLong)
	}
}

func TestShouldUseBotBiasedDrawRespectsThreshold(t *testing.T) {
	gameID := uuid.MustParse("00000000-0000-0000-0000-000000000042")
	threshold := botBiasStartDrawCount(gameID)

	if shouldUseBotBiasedDraw(gameID, threshold-1) {
		t.Fatalf("expected no bot bias before threshold %d", threshold)
	}
	if !shouldUseBotBiasedDraw(gameID, threshold) {
		t.Fatalf("expected bot bias at threshold %d", threshold)
	}
	if !shouldUseBotBiasedDraw(gameID, threshold+5) {
		t.Fatalf("expected bot bias after threshold %d", threshold)
	}
}
