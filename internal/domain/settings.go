package domain

import "time"

// AppSettings holds operator-tunable knobs edited from the admin dashboard.
// Single row (app_settings table). Extend as more settings are added.
type AppSettings struct {
	MinDeposit      float64   `json:"min_deposit" db:"min_deposit"`
	ReferralEnabled bool      `json:"referral_enabled" db:"referral_enabled"`
	ReferralAmount  float64   `json:"referral_amount" db:"referral_amount"`
	// MaintenanceMode puts the player Mini App into "we'll be right back" mode:
	// the frontend shows a maintenance screen and the API rejects player actions
	// (join/deposit/withdraw/…) with 503, while the admin dashboard stays fully
	// usable. MaintenanceMessage is an optional operator note shown to players.
	MaintenanceMode    bool      `json:"maintenance_mode" db:"maintenance_mode"`
	MaintenanceMessage string    `json:"maintenance_message" db:"maintenance_message"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// UpdateAppSettingsRequest is the admin payload to change settings. Pointers so a
// partial update leaves untouched fields alone.
type UpdateAppSettingsRequest struct {
	MinDeposit         *float64 `json:"min_deposit,omitempty"`
	ReferralEnabled    *bool    `json:"referral_enabled,omitempty"`
	ReferralAmount     *float64 `json:"referral_amount,omitempty"`
	MaintenanceMode    *bool    `json:"maintenance_mode,omitempty"`
	MaintenanceMessage *string  `json:"maintenance_message,omitempty"`
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
