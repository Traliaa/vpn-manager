// Package bot provides a Telegram bot for managing VPN routing and monitoring.
package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/routing"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/service"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// Bot manages Telegram bot polling and command handling.
type Bot struct {
	api       *tgbotapi.BotAPI
	manager   *vpn.Manager
	activator *routing.Activator
	svc       *service.Service
	queries   *db.Queries
	logger    *zap.Logger

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// New creates a new Bot. If token is empty, creates a no-op bot.
func New(token string, manager *vpn.Manager, activator *routing.Activator,
	svc *service.Service, queries *db.Queries, logger *zap.Logger) *Bot {

	b := &Bot{
		manager:   manager,
		activator: activator,
		svc:       svc,
		queries:   queries,
		logger:    logger.Named("telegram-bot"),
		stopCh:    make(chan struct{}),
	}

	if token != "" {
		api, err := tgbotapi.NewBotAPI(token)
		if err != nil {
			b.logger.Warn("failed to create bot API, bot disabled", zap.Error(err))
			return b
		}
		b.api = api
		b.logger.Info("telegram bot initialized", zap.String("username", api.Self.UserName))
	} else {
		b.logger.Info("telegram bot token not set, bot disabled")
	}

	return b
}

// Start begins polling for bot updates.
func (b *Bot) Start(ctx context.Context) error {
	if b.api == nil {
		b.logger.Info("telegram bot is disabled (no API client)")
		return nil
	}

	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil
	}
	b.running = true
	b.mu.Unlock()

	b.logger.Info("telegram bot started polling")

	go func() {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := b.api.GetUpdatesChan(u)

		for {
			select {
			case update := <-updates:
				if update.Message == nil || !update.Message.IsCommand() {
					continue
				}
				b.handleCommand(update.Message)
			case <-b.stopCh:
				b.logger.Info("telegram bot polling stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop gracefully stops the bot polling.
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		close(b.stopCh)
		b.running = false
	}
}

// handleCommand dispatches a command message.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	var reply string

	switch msg.Command() {
	case "start", "help":
		reply = b.cmdHelp()
	case "status":
		reply = b.cmdStatus(msg)
	case "routing":
		reply = b.cmdRouting()
	case "profiles":
		reply = b.cmdProfiles()
	case "activate":
		reply = b.cmdActivate(msg)
	case "deactivate":
		reply = b.cmdDeactivate()
	case "sync":
		reply = b.cmdSync()
	default:
		reply = fmt.Sprintf("Неизвестная команда: /%s\n\n%s", msg.Command(), b.cmdHelp())
	}

	b.sendReply(msg.Chat.ID, reply)
}

// sendReply sends a text message to the given chat.
func (b *Bot) sendReply(chatID int64, text string) {
	if b.api == nil {
		return
	}
	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = "MarkdownV2"
	if _, err := b.api.Send(reply); err != nil {
		b.logger.Warn("failed to send telegram message",
			zap.Int64("chat_id", chatID),
			zap.Error(err),
		)
	}
}

// cmdHelp returns the help text with all available commands.
func (b *Bot) cmdHelp() string {
	return escapeMD(`📋 *VPN Manager — доступные команды*

/status — статус всех VPN провайдеров
/routing — текущий статус маршрутизации
/profiles — список профилей маршрутизации
/activate N — активировать профиль по номеру
/deactivate — деактивировать маршрутизацию
/sync — синхронизировать провайдеров из БД
/help — это сообщение`)
}

// cmdStatus returns the status of all VPN providers.
func (b *Bot) cmdStatus(msg *tgbotapi.Message) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statuses := b.manager.AllStatuses(ctx)
	if len(statuses) == 0 {
		return escapeMD("⚠️ Нет активных провайдеров")
	}

	var sb strings.Builder
	sb.WriteString("📡 *Статус провайдеров*\n\n")

	for _, s := range statuses {
		stateIcon := mapStateIcon(s.State)
		uptime := ""
		if s.State == vpn.StateUp && s.Uptime > 0 {
			uptime = fmt.Sprintf(" (up %s)", formatDuration(s.Uptime))
		}

		sb.WriteString(fmt.Sprintf("%s *%s* — %s%s\n",
			stateIcon,
			escapeMD(s.Name),
			escapeMD(string(s.State)),
			escapeMD(uptime),
		))

		if s.State == vpn.StateUp {
			tx := formatBytes(s.TxBytes)
			rx := formatBytes(s.RxBytes)
			sb.WriteString(fmt.Sprintf("  └ TX: %s │ RX: %s\n",
				escapeMD(tx), escapeMD(rx)))
		}
	}

	return sb.String()
}

// cmdRouting returns the current routing status.
func (b *Bot) cmdRouting() string {
	profile := b.activator.ActiveProfile()
	if profile == nil {
		return escapeMD("🔓 Маршрутизация не активна — весь трафик идёт напрямую")
	}

	return fmt.Sprintf(escapeMD("🔒 *Маршрутизация активна*\n\nПрофиль: *%s*\nОписание: %s"),
		escapeMD(profile.Name),
		escapeMD(profile.Description.String),
	)
}

// cmdProfiles returns a numbered list of all routing profiles.
func (b *Bot) cmdProfiles() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	profiles, err := b.queries.ListProfiles(ctx)
	if err != nil {
		return escapeMD(fmt.Sprintf("❌ Ошибка загрузки профилей: %s", err))
	}

	if len(profiles) == 0 {
		return escapeMD("📋 Профили не найдены. Создайте профиль через Web UI.")
	}

	var sb strings.Builder
	sb.WriteString("📋 *Профили маршрутизации*\n\n")

	activeID := b.activator.ActiveProfileID()

	for i, p := range profiles {
		active := ""
		if p.ID == activeID {
			active = " ✅ *активен*"
		}
		defaultTag := ""
		if p.IsDefault {
			defaultTag = " (default)"
		}
		desc := ""
		if p.Description.Valid {
			desc = " — " + p.Description.String
		}
		sb.WriteString(fmt.Sprintf("%d\\. *%s*%s%s\n  └ ID: `%s`%s\n",
			i+1,
			escapeMD(p.Name),
			escapeMD(defaultTag),
			escapeMD(desc),
			escapeMD(shortUUID(p.ID)),
			active,
		))
	}

	sb.WriteString(fmt.Sprintf("\nВсего: %d профилей", len(profiles)))

	return sb.String()
}

// cmdActivate activates a profile by its number from the profiles list.
func (b *Bot) cmdActivate(msg *tgbotapi.Message) string {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return escapeMD("❌ Укажите номер профиля: `/activate 1`\nСписок профилей: /profiles")
	}

	num, err := strconv.Atoi(args)
	if err != nil || num < 1 {
		return escapeMD("❌ Некорректный номер. Используйте: `/activate 1`")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	profiles, err := b.queries.ListProfiles(ctx)
	if err != nil {
		return escapeMD(fmt.Sprintf("❌ Ошибка загрузки профилей: %s", err))
	}

	if num > len(profiles) {
		return escapeMD(fmt.Sprintf("❌ Профиль #%d не найден. Всего профилей: %d", num, len(profiles)))
	}

	profile := profiles[num-1]

	if err := b.activator.Activate(ctx, profile.ID); err != nil {
		return escapeMD(fmt.Sprintf("❌ Ошибка активации профиля %q: %s", profile.Name, err))
	}

	return escapeMD(fmt.Sprintf("✅ Профиль *%s* активирован", profile.Name))
}

// cmdDeactivate deactivates the current routing profile.
func (b *Bot) cmdDeactivate() string {
	if b.activator.ActiveProfile() == nil {
		return escapeMD("⚠️ Маршрутизация уже не активна")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.activator.Deactivate(ctx); err != nil {
		return escapeMD(fmt.Sprintf("❌ Ошибка деактивации: %s", err))
	}

	return escapeMD("✅ Маршрутизация деактивирована — весь трафик идёт напрямую")
}

// cmdSync triggers a provider sync from the database.
func (b *Bot) cmdSync() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := b.svc.SyncProviders(ctx); err != nil {
		return escapeMD(fmt.Sprintf("❌ Ошибка синхронизации: %s", err))
	}

	count := len(b.manager.List())
	return escapeMD(fmt.Sprintf("✅ Провайдеры синхронизированы\nАктивно провайдеров: %d", count))
}

// --- helpers ---

// escapeMD escapes special characters for Telegram MarkdownV2.
func escapeMD(s string) string {
	replacements := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	for _, ch := range replacements {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
	}
	return s
}

// mapStateIcon returns an emoji icon for a given interface state.
func mapStateIcon(s vpn.InterfaceState) string {
	switch s {
	case vpn.StateUp:
		return "✅"
	case vpn.StateDown:
		return "❌"
	case vpn.StateError:
		return "⚠️"
	default:
		return "❓"
	}
}

// formatDuration formats a time.Duration into a human-readable string.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(b int64) string {
	if b >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	}
	if b >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	}
	if b >= 1<<10 {
		return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

// shortUUID returns the first 8 characters of a UUID string.
func shortUUID(id any) string {
	return fmt.Sprintf("%s", id)[:8]
}
