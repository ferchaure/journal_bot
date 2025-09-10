package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/pelletier/go-toml"
)

type Config struct {
	ApiToken      string  `toml:"api_token"`
	Users         []int64 `toml:"users"`
	JournalFolder string  `toml:"journal_folder"`
}

const (
	CommandJournal = "/journal"
	CommandStop    = "/stop"
	CommandStatus  = "/status"
)

var cfg Config
var chat_commands_status = make(map[int64]map[string]string)
var events_count int64 = 0

func main() {

	data, err := os.ReadFile("config.toml")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		fmt.Println("Error parsing TOML:", err)
		os.Exit(1)
	}
	if !strings.HasSuffix(cfg.JournalFolder, "/") {
		cfg.JournalFolder += "/"
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
		bot.WithCallbackQueryDataHandler("journal_", bot.MatchTypePrefix, journalcallbackHandler),
	}

	b, err := bot.New(cfg.ApiToken, opts...)
	if err != nil {
		panic(err)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, CommandJournal, bot.MatchTypeExact, journalHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, CommandStop, bot.MatchTypeExact, stopHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, CommandStatus, bot.MatchTypeExact, statusHandler)

	commands := []models.BotCommand{
		{
			Command:     CommandJournal,
			Description: "To upload text to journal",
		},
		{
			Command:     CommandStatus,
			Description: "returns number of events from the bot boot.",
		},
		{
			Command:     CommandStop,
			Description: "stops the bot",
		}}

	for _, v := range cfg.Users {
		scope := models.BotCommandScopeChat{ChatID: v}
		b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: commands,
			Scope:    &scope,
		})
	}
	fmt.Println("Running bot...")
	b.Start(ctx)
}

func isUser(val int64) bool {
	atomic.AddInt64(&events_count, 1)
	for _, v := range cfg.Users {
		if v == val {
			return true
		}
	}
	return false
}

func statusHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("Events since boot: %d", atomic.LoadInt64(&events_count)),
	})
}

func stopHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Stopping bot...",
	})
	os.Exit(0)
}

func journalHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	path := journal_filename(update.Message.Date)
	chat_commands_status[update.Message.Chat.ID] = make(map[string]string)
	chat_commands_status[update.Message.Chat.ID]["command"] = CommandJournal
	chat_commands_status[update.Message.Chat.ID]["path"] = path

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("Send text to add to journal (%s):", path),
		ParseMode: models.ParseModeMarkdown,
	})

}

func journal_filename(unixTime int) string {
	// Get current date
	// return in format YYYY-MM-DD
	t := time.Unix(int64(unixTime), 0)

	// Format as YYYY-MM-DD
	return cfg.JournalFolder + t.Format("2006-01-02") + ".md"
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	if chat_commands_status[update.Message.Chat.ID] == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Sent a command first",
		})
		return
	}

	if chat_commands_status[update.Message.Chat.ID]["command"] == CommandJournal {
		path := chat_commands_status[update.Message.Chat.ID]["path"]
		chat_commands_status[update.Message.Chat.ID]["text"] = update.Message.Text
		if _, err := os.Stat(path); err == nil {
			kb := &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: "replace", CallbackData: "journal_replace"},
						{Text: "append", CallbackData: "journal_append"},
					},
					{
						{Text: "read", CallbackData: "journal_read"},
						{Text: "cancel", CallbackData: "journal_cancel"},
					},
				},
			}

			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      update.Message.Chat.ID,
				Text:        "File already exists. Select action:",
				ReplyMarkup: kb,
			})
			fmt.Printf("Asking action about: %s \n ", path)
			return
		}

		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			report_error(ctx, b, update.Message.Chat.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(update.Message.Text); err != nil {
			report_error(ctx, b, update.Message.Chat.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Texto añadido al journal",
		})
		delete(chat_commands_status, update.Message.Chat.ID)
	}

	// b.SendMessage(ctx, &bot.SendMessageParams{
	// 	ChatID: update.Message.Chat.ID,
	// 	Text:   time.Unix(int64(update.Message.Date), 0).Format("2006-01-02 15:04:05"),
	// })
}

func journalcallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	// answering callback query first to let Telegram know that we received the callback query,
	// and we're handling it. Otherwise, Telegram might retry sending the update repetitively
	// as it thinks the callback query doesn't reach to our application. learn more by
	// reading the footnote of the https://core.telegram.org/bots/api#callbackquery type.
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})
	path := chat_commands_status[update.CallbackQuery.From.ID]["path"]
	switch update.CallbackQuery.Data {
	case "journal_cancel":
		delete(chat_commands_status, update.CallbackQuery.From.ID)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   "Operation cancelled",
		})
	case "journal_read":
		data, err := os.ReadFile(path)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   string(data),
		})
	case "journal_replace":
		f, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(chat_commands_status[update.CallbackQuery.From.ID]["text"]); err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		delete(chat_commands_status, update.CallbackQuery.From.ID)
	case "journal_append":
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString("\n\n" + chat_commands_status[update.CallbackQuery.From.ID]["text"]); err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   "Texto añadido al journal",
		})
		delete(chat_commands_status, update.CallbackQuery.From.ID)
	}
}

func report_error(ctx context.Context, b *bot.Bot, id int64, err error) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: id,
		Text:   "*Error*:\n" + err.Error(),
	})
	fmt.Println("*Error*:\n", err)
}
