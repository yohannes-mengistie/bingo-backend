package domain

// LoginRequest represents the data needed for admin login
type LoginRequest struct {
	TelegramID int64  `json:"telegram_id" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// LoginResponse represents the response after successful login
type LoginResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

// TelegramAuthRequest carries the signed initData string from a Telegram Mini App.
type TelegramAuthRequest struct {
	InitData string `json:"init_data" binding:"required"`
}

// CreateAdminRequest promotes an existing user to admin and sets a password.
type CreateAdminRequest struct {
	TelegramID int64  `json:"telegram_id" binding:"required"`
	Password   string `json:"password" binding:"required,min=8"`
	SecretCode string `json:"secret_code" binding:"required"`
}
