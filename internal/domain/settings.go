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
