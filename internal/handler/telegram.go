package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/bingo/backend/pkg/telegram"
	"github.com/bingo/backend/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TelegramHandler handles Telegram bot webhook updates. The bot is the only way
// users register (it captures the phone number a Mini App cannot read), so this
// is the registration gateway in front of the Mini App.
type TelegramHandler struct {
	userUseCase   *usecase.UserUseCase
	promoRepo     domain.PromoRepository
	bot           *telegram.Bot
	webhookSecret string
	miniAppBase   string // Mini App origin, no trailing slash
	appVersion    string // per-deploy cache-buster (see appURL)

	// promoWaiting marks chats whose NEXT message should be treated as a
	// promo-code attempt (set when the user taps the promo menu button).
	// In-memory is fine: worst case after a restart the user just taps the
	// button again. Entries expire after promoWaitTTL.
	promoMu      sync.Mutex
	promoWaiting map[int64]time.Time
}

// promoWaitTTL is how long a "send me your promo code" prompt stays armed.
const promoWaitTTL = 5 * time.Minute

// NewTelegramHandler creates a new Telegram webhook handler.
func NewTelegramHandler(userUseCase *usecase.UserUseCase, promoRepo domain.PromoRepository, bot *telegram.Bot, webhookSecret, miniAppURL string) *TelegramHandler {
	base := strings.TrimRight(miniAppURL, "/")
	version := miniAppCacheVersion()
	return &TelegramHandler{
		userUseCase:   userUseCase,
		promoRepo:     promoRepo,
		bot:           bot,
		webhookSecret: webhookSecret,
		// appURL bakes a per-deploy cache-buster into every Mini App URL.
		// Telegram caches the web app per-URL and ignores our no-cache
		// headers, so without a changing query string players keep seeing the
		// previous build after a deploy. Resolved once at startup (stable
		// within a deploy, so repeated opens still hit Telegram's cache;
		// changes on the next deploy).
		miniAppBase:  base,
		appVersion:   version,
		promoWaiting: make(map[int64]time.Time),
	}
}

// armPromo marks chatID so its next message is read as a promo code.
func (h *TelegramHandler) armPromo(chatID int64) {
	h.promoMu.Lock()
	defer h.promoMu.Unlock()
	h.promoWaiting[chatID] = time.Now().Add(promoWaitTTL)
}

// disarmPromo reports whether chatID was armed (and clears it).
func (h *TelegramHandler) disarmPromo(chatID int64) bool {
	h.promoMu.Lock()
	defer h.promoMu.Unlock()
	deadline, ok := h.promoWaiting[chatID]
	if !ok {
		return false
	}
	delete(h.promoWaiting, chatID)
	return time.Now().Before(deadline)
}

// appURL builds a deep link into the Mini App (e.g. appURL("/wallet")) with
// the per-deploy cache-buster attached. The SPA serves every path, so a menu
// button can land the player directly on the right screen.
func (h *TelegramHandler) appURL(path string) string {
	return withCacheVersion(h.miniAppBase+path, h.appVersion)
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
		h.handleMenuText(c, msg)
	}

	c.Status(http.StatusOK)
}

// Main-menu button labels — the persistent reply keyboard every registered
// user sees (mirrors the layout in mainMenu). web_app buttons open the Mini
// App directly; plain buttons echo their label back as a message and are
// routed by exact match in handleMenuText.
const (
	btnPlay     = "🎮 ቢንጎ ተጫወት"
	btnPromo    = "🎁 ፕሮሞ ኮድ"
	btnDeposit  = "💰 ገቢ ለማድረግ"
	btnWithdraw = "💸 ወጪ ለማድረግ"
	btnInvite   = "🔗 ጋብዝ & አግኝ"
	btnProfile  = "👤 ፕሮፋይል & ሂሳብ"
	btnHelp     = "🆘 እርዳታ"
	btnLanguage = "🌍 ቋንቋ / Language"
	btnAgent    = "📢 አጀንት ፕሮሞተር"
)

// mainMenu is the persistent reply keyboard (the button grid pinned above the
// system keyboard). All buttons are PLAIN text buttons: Telegram does not pass
// initData to Mini Apps launched from a reply-keyboard web_app button, so the
// app could not authenticate the user ("open inside Telegram" guard). Instead
// each tap is answered instantly with an INLINE web_app button (which does
// carry initData) that opens the Mini App on the matching screen.
func (h *TelegramHandler) mainMenu() *telegram.ReplyMarkup {
	txt := func(text string) telegram.KeyboardButton {
		return telegram.KeyboardButton{Text: text}
	}
	return &telegram.ReplyMarkup{
		Keyboard: [][]telegram.KeyboardButton{
			{txt(btnPlay), txt(btnPromo)},
			{txt(btnDeposit), txt(btnWithdraw)},
			{txt(btnInvite), txt(btnProfile)},
			{txt(btnHelp), txt(btnLanguage)},
			{txt(btnAgent)},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

// appButton replies with one inline web_app button that opens the Mini App at
// path — the only launch method from chat that authenticates the user.
func (h *TelegramHandler) appButton(chatID int64, text, buttonLabel, path string) {
	h.reply(chatID, text, &telegram.ReplyMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: buttonLabel, WebApp: &telegram.WebAppInfo{URL: h.appURL(path)}},
		}},
	})
}

// handleMenuText routes taps on the plain (non-web_app) menu buttons, and
// falls back to re-showing the menu for anything unrecognized. Unregistered
// users are funneled into the /start registration flow instead.
func (h *TelegramHandler) handleMenuText(c *gin.Context, msg *telegram.Message) {
	user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), msg.From.ID)
	if err != nil || user == nil {
		h.handleStart(c, msg)
		return
	}

	// A tap on the promo button arms the chat: the NEXT message is the code.
	text := strings.TrimSpace(msg.Text)
	if text != btnPromo && h.disarmPromo(msg.Chat.ID) {
		h.redeemPromo(c, msg, user.ID, text)
		return
	}

	switch text {
	case btnPlay:
		h.appButton(msg.Chat.ID, "🎮 ለመጫወት ከታች ይንኩ 👇", "🎮 ቢንጎ ተጫወት / Play", "/")
	case btnDeposit:
		h.appButton(msg.Chat.ID, "💰 ገንዘብ ለማስገባት ከታች ይንኩ 👇", "💰 ቦርሳ ክፈት / Open Wallet", "/wallet")
	case btnWithdraw:
		h.appButton(msg.Chat.ID, "💸 ገንዘብ ለማውጣት ከታች ይንኩ 👇", "💸 ቦርሳ ክፈት / Open Wallet", "/wallet")
	case btnInvite:
		h.appButton(msg.Chat.ID, "🔗 ጓደኞችዎን ጋብዘው ቦነስ ያግኙ 👇", "🔗 ጋብዝ & አግኝ / Invite", "/referral")
	case btnProfile:
		h.appButton(msg.Chat.ID, "👤 ፕሮፋይልዎን እና ሂሳብዎን ለማየት ከታች ይንኩ 👇", "👤 ፕሮፋይል / Profile", "/profile")
	case btnPromo:
		h.armPromo(msg.Chat.ID)
		h.reply(msg.Chat.ID,
			"🎁 የፕሮሞ ኮድዎን አሁን ይላኩ 👇\n\nSend your promo code now 👇",
			nil)
	case btnHelp:
		h.reply(msg.Chat.ID,
			"🆘 እርዳታ / Help\n\n"+
				"🎮 ለመጫወት፦ «"+btnPlay+"» ይንኩ፣ ካርድ ይምረጡ — ጨዋታው ካርታዎን በራስ-ሰር ያደምቃል።\n"+
				"💰 ገቢ ለማድረግ፦ በቴሌብር ወይም ኤም-ፔሳ ገንዘብ ልከው ደረሰኙን በመተግበሪያው ያስገቡ።\n"+
				"💸 ወጪ ለማድረግ፦ ከቦርሳ ገጹ ላይ ይጠይቁ።\n\n"+
				"ችግር ካጋጠመዎ ከታች ያለውን ይንኩ 👇",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "🛟 ችግር ሪፖርት / Report a problem", WebApp: &telegram.WebAppInfo{URL: h.appURL("/report")}},
			}}})
	case btnLanguage:
		h.reply(msg.Chat.ID,
			"🌍 ቋንቋ (አማርኛ / English) በመተግበሪያው ውስጥ ከፕሮፋይል ገጽ መቀየር ይችላሉ።\n\nYou can switch the app language from the Profile page.",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "👤 ፕሮፋይል / Profile", WebApp: &telegram.WebAppInfo{URL: h.appURL("/profile")}},
			}}})
	case btnAgent:
		h.reply(msg.Chat.ID,
			"📢 አጀንት/ፕሮሞተር ለመሆን ይፈልጋሉ?\n\nበመተግበሪያው ውስጥ «ችግር ሪፖርት» ገጹን ተጠቅመው መልዕክት ይላኩልን — እናገኝዎታለን።",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "✉️ መልዕክት ላክ / Contact us", WebApp: &telegram.WebAppInfo{URL: h.appURL("/report")}},
			}}})
	default:
		h.reply(msg.Chat.ID,
			"እባክዎ ከታች ካለው ማውጫ ይምረጡ 👇\nPlease choose from the menu below 👇",
			h.mainMenu())
	}
}

// redeemPromo applies a promo code sent after the promo button was tapped and
// answers every outcome distinctly (in Amharic + English).
func (h *TelegramHandler) redeemPromo(c *gin.Context, msg *telegram.Message, userID uuid.UUID, code string) {
	if h.promoRepo == nil {
		h.reply(msg.Chat.ID, "🎁 የፕሮሞ ኮድ ስርዓት በአሁኑ ጊዜ አይገኝም። / Promo codes are currently unavailable.", h.mainMenu())
		return
	}

	amount, err := h.promoRepo.Redeem(c.Request.Context(), code, userID)
	switch {
	case err == nil:
		h.reply(msg.Chat.ID,
			fmt.Sprintf("🎉 እንኳን ደስ አለዎት! %.0f ብር ቦነስ ወደ ቦርሳዎ ተጨምሯል።\n\nCongratulations! A %.0f birr bonus was added to your wallet. 💰", amount, amount),
			h.mainMenu())
	case errors.Is(err, domain.ErrPromoAlreadyRedeemed):
		h.reply(msg.Chat.ID, "ℹ️ ይህን ኮድ ከዚህ በፊት ተጠቅመዋል።\nYou have already used this code.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoExpired), errors.Is(err, domain.ErrPromoInactive):
		h.reply(msg.Chat.ID, "⌛ ይቅርታ፣ የዚህ ኮድ ጊዜ አልፎበታል።\nSorry, this code is no longer valid.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoExhausted):
		h.reply(msg.Chat.ID, "😔 ይቅርታ፣ የዚህ ኮድ ተጠቃሚዎች ብዛት ተሟልቷል።\nSorry, this code has reached its redemption limit.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoNotFound):
		h.reply(msg.Chat.ID, "❌ ኮዱ አልተገኘም። እባክዎ በትክክል መጻፉን ያረጋግጡና «"+btnPromo+"»ን ነክተው እንደገና ይሞክሩ።\nCode not found — check the spelling and try again.", h.mainMenu())
	default:
		log.Printf("[telegram] promo redeem failed for user %s: %v", userID, err)
		h.reply(msg.Chat.ID, "⚠️ የሆነ ችግር ተፈጥሯል፣ እባክዎ ቆየት ብለው ይሞክሩ።\nSomething went wrong — please try again later.", h.mainMenu())
	}
}

// handleStart greets the user. If already registered, it shows the persistent
// main menu straight away; otherwise it asks them to share their phone number.
func (h *TelegramHandler) handleStart(c *gin.Context, msg *telegram.Message) {
	if user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), msg.From.ID); err == nil && user != nil {
		h.reply(msg.Chat.ID,
			"እንኳን ደህና መጡ፣ "+user.FirstName+"! 🎉\nከታች ያለውን ማውጫ ይጠቀሙ 👇\n\nWelcome back! Use the menu below 👇",
			h.mainMenu())
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
		"ምዝገባዎ ተጠናቋል! 🎉 ከታች ያለውን ማውጫ ይጠቀሙ 👇\n\nYou're all set! Use the menu below 👇",
		h.mainMenu())
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
