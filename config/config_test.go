package config

import "testing"

func TestBotWalletFloatDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret-with-at-least-32-characters")
	t.Setenv("BOT_WALLET_FLOAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Bots.WalletFloat != 1_000_000 {
		t.Fatalf("Bots.WalletFloat = %v, want 1000000", cfg.Bots.WalletFloat)
	}
}

func TestBotWalletFloatOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret-with-at-least-32-characters")
	t.Setenv("BOT_WALLET_FLOAT", "250000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Bots.WalletFloat != 250_000 {
		t.Fatalf("Bots.WalletFloat = %v, want 250000", cfg.Bots.WalletFloat)
	}
}
