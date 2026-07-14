package handler

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/bingo/backend/pkg/telegram"
	"github.com/bingo/backend/pkg/utils"
	"github.com/gin-gonic/gin"
)

// TelegramHandler handles Telegram bot webhook updates. The bot is the only way
// users register (it captures the phone number a Mini App cannot read), so this
// is the registration gateway in front of the Mini App.
type TelegramHandler struct {
	userUseCase   *usecase.UserUseCase
	bot           *telegram.Bot
	webhookSecret string
	miniAppURL    string
}

// NewTelegramHandler creates a new Telegram webhook handler.
func NewTelegramHandler(userUseCase *usecase.UserUseCase, bot *telegram.Bot, webhookSecret, miniAppURL string) *TelegramHandler {
	return &TelegramHandler{
		userUseCase:   userUseCase,
		bot:           bot,
		webhookSecret: webhookSecret,
		// Bake a per-deploy cache-buster into the Mini App URL. Telegram caches
		// the web app per-URL and ignores our no-cache headers, so without a
		// changing query string players keep seeing the previous build after a
		// deploy. Resolved once at startup (stable within a deploy, so repeated
		// opens still hit Telegram's cache; changes on the next deploy).
		miniAppURL: withCacheVersion(miniAppURL, miniAppCacheVersion()),
	}
}

// miniAppCacheVersion returns a token that changes on every deploy. On Render
// that's the deployed commit (RENDER_GIT_COMMIT); locally it falls back to the
// process start time, so a restart still busts the cache.
func miniAppCacheVersion() string {
	if c := os.Getenv("RENDER_GIT_COMMIT"); c != "" {
		if len(c) > 8 {
			c = c[:8]
		}
		return c
	}
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// withCacheVersion appends ?v=<version> (or &v= when the URL already has a
// query) so Telegram treats each deploy's URL as new. A blank base or version
// is returned unchanged.
func withCacheVersion(rawURL, version string) string {
	if rawURL == "" || version == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "v=" + version
}

// Webhook handles POST /telegram/webhook — the endpoint registered with
// Telegram via setWebhook. It ALWAYS returns 200 quickly (even on internal
// errors) so Telegram does not retry and back off the bot.
func (h *TelegramHandler) Webhook(c *gin.Context) {
	// Verify the request really came from Telegram via the shared secret.
	if h.webhookSecret != "" {
		if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != h.webhookSecret {
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var update telegram.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		// Malformed body — ack so Telegram stops resending, but log it.
		log.Printf("[telegram] failed to parse update: %v", err)
		c.Status(http.StatusOK)
		return
	}

	msg := update.Message
	if msg == nil || msg.From == nil {
		c.Status(http.StatusOK) // nothing actionable (edited message, etc.)
		return
	}

	switch {
	case msg.Contact != nil:
		h.handleContact(c, msg)
	case strings.HasPrefix(msg.Text, "/start"):
		h.handleStart(c, msg)
	default:
		h.reply(msg.Chat.ID, "Send /start to begin. 👋", nil)
	}

	c.Status(http.StatusOK)
}

// handleStart greets the user. If already registered, it shows the Play button
// straight away; otherwise it asks them to share their phone number.
func (h *TelegramHandler) handleStart(c *gin.Context, msg *telegram.Message) {
	if user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), msg.From.ID); err == nil && user != nil {
		h.reply(msg.Chat.ID,
			"Welcome back, "+user.FirstName+"! 🎉 Tap below to play.",
			telegram.PlayButton("🎮 Play Bingo", h.miniAppURL))
		return
	}

	h.reply(msg.Chat.ID,
		"Welcome to Edl Bingo! · እንኳን ወደ እድል ቢንጎ በደህና መጡ! 🎯\n\nTo create your account, tap the button below to share your phone number.",
		telegram.ContactRequestKeyboard("📱 Share my phone number"))
}

// handleContact registers the user from their shared contact, then shows Play.
func (h *TelegramHandler) handleContact(c *gin.Context, msg *telegram.Message) {
	contact := msg.Contact

	// Only accept the user's OWN contact, not one forwarded from someone else.
	if contact.UserID != 0 && contact.UserID != msg.From.ID {
		h.reply(msg.Chat.ID, "Please share *your own* phone number using the button.", nil)
		return
	}

	if !utils.IsEthiopianMobile(contact.PhoneNumber) {
		h.reply(msg.Chat.ID, "Sorry, only Ethiopian phone numbers (+251) are supported.", nil)
		return
	}

	var lastName *string
	if ln := strings.TrimSpace(msg.From.LastName); ln != "" {
		lastName = &ln
	}

	_, _, err := h.userUseCase.CreateUser(c.Request.Context(), domain.CreateUserRequest{
		TelegramID: msg.From.ID,
		FirstName:  msg.From.FirstName,
		LastName:   lastName,
		Phone:      contact.PhoneNumber,
	})

	// Treat "already registered" as success — the user just wants to play.
	if err != nil && !isAlreadyRegistered(err) {
		log.Printf("[telegram] register failed for tg_id=%d: %v", msg.From.ID, err)
		h.reply(msg.Chat.ID, "Something went wrong creating your account. Please try /start again.", nil)
		return
	}

	h.reply(msg.Chat.ID,
		"You're all set! 🎉 Tap below to play.",
		telegram.PlayButton("🎮 Play Bingo", h.miniAppURL))
}

// reply sends a message and logs (but swallows) any send error.
func (h *TelegramHandler) reply(chatID int64, text string, markup *telegram.ReplyMarkup) {
	if err := h.bot.SendMessage(chatID, text, markup); err != nil {
		log.Printf("[telegram] sendMessage to chat %d failed: %v", chatID, err)
	}
}

// isAlreadyRegistered reports whether err is a duplicate-user error from
// CreateUser (same telegram ID or phone already exists).
func isAlreadyRegistered(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}
