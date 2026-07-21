package domain

import "time"

// AppSettings holds operator-tunable knobs edited from the admin dashboard.
// Single row (app_settings table). Extend as more settings are added.
type AppSettings struct {
	MinDeposit float64   `json:"min_deposit" db:"min_deposit"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// UpdateAppSettingsRequest is the admin payload to change settings. Pointers so a
// partial update leaves untouched fields alone.
type UpdateAppSettingsRequest struct {
	MinDeposit *float64 `json:"min_deposit,omitempty"`
}

// WithdrawalRollbackResult reports how a rejected withdrawal was split back: the
// genuine (deposit/winnings-backed) part returned to withdrawable cash, and the
// remainder — money the player never earned by playing — returned as play-only
// bonus instead. See WalletUseCase.RejectWithdrawalToBonus.
type WithdrawalRollbackResult struct {
	Amount       float64 `json:"amount"`        // the withdrawal that was rolled back
	RealRefunded float64 `json:"real_refunded"` // returned to withdrawable balance
	BonusGranted float64 `json:"bonus_granted"` // returned as play-only bonus
}
