package bingo

import "testing"

func TestValidateBingo_FourCornersWins(t *testing.T) {
	card := GenerateCard(1)

	marked := []int{
		card.Numbers[0][0],
		card.Numbers[0][4],
		card.Numbers[4][0],
		card.Numbers[4][4],
	}

	if !ValidateBingo(card, marked) {
		t.Fatalf("expected four corners to be a valid winning pattern")
	}
}

func TestValidateBingo_ThreeCornersDoesNotWin(t *testing.T) {
	card := GenerateCard(1)

	marked := []int{
		card.Numbers[0][0],
		card.Numbers[0][4],
		card.Numbers[4][0],
	}

	if ValidateBingo(card, marked) {
		t.Fatalf("expected three corners to be invalid")
	}
}

