package handler

import (
	"net/http"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserHandler struct {
	userUseCase *usecase.UserUseCase
}

// NewUserHandler creates a new user handler
func NewUserHandler(userUseCase *usecase.UserUseCase) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
	}
}

// Register handles the POST /user/register endpoint
func (h *UserHandler) Register(c *gin.Context) {
	var req domain.CreateUserRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, wallet, err := h.userUseCase.CreateUser(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user with this telegram ID already exists" ||
			err.Error() == "user with this phone number already exists" {
			statusCode = http.StatusConflict
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User and wallet created successfully",
		"user":   user,
		"wallet": wallet,
	})
}

// FindByTelegramID handles the GET /user/telegram/:telegram_id endpoint
func (h *UserHandler) FindByTelegramID(c *gin.Context) {
	var uri struct {
		TelegramID int64 `uri:"telegram_id" binding:"required"`
	}

	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid telegram ID",
		})
		return
	}

	user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), uri.TelegramID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// FindByPhone handles the GET /user/phone endpoint
func (h *UserHandler) FindByPhone(c *gin.Context) {
	var req struct {
		Phone string `form:"phone" binding:"required"`
	}

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "phone parameter is required",
		})
		return
	}

	user, err := h.userUseCase.FindUserByPhone(c.Request.Context(), req.Phone)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// FindByReferralCode handles the GET /user/referral/:referral_code endpoint
func (h *UserHandler) FindByReferralCode(c *gin.Context) {
	referralCode := c.Param("referral_code")
	if referralCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "referral_code is required",
		})
		return
	}

	user, err := h.userUseCase.FindUserByReferralCode(c.Request.Context(), referralCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}


// GetAllUsers handles the GET /admin/users endpoint
func (h *UserHandler) GetAllUsers(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	users, err := h.userUseCase.GetAllUsers(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"count": len(users),
		"limit": limit,
		"offset": offset,
	})
}

// UpdateUserName handles the PUT /user/:user_id/name endpoint
func (h *UserHandler) UpdateUserName(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	var req domain.UpdateUserNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, err := h.userUseCase.UpdateUserName(c.Request.Context(), userID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user not found" {
			statusCode = http.StatusNotFound
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User name updated successfully",
		"user":    user,
	})
}

