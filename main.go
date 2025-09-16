// agregar al dict que sea por message id que que responda con las opciones para permitir paralelo
// usar todo colas o algo safe
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	CommandStop   = "/stop"
	CommandStatus = "/status"
	CommandHelp   = "/help"
)

const (
	JournalCallback = "journal_"
	FileCallback    = "file_"
)

const (
	JournalAdd    = JournalCallback + "add"
	JournalAppend = JournalCallback + "append"
)

const (
	FileReplace = FileCallback + "replace"
	FileAppend  = FileCallback + "append"
	FileRead    = FileCallback + "read"
	FileCancel  = FileCallback + "cancel"
)

type user_data struct { //just remember the last message of the user
	Content           string
	File              string
	OriginalMessageID int
	CurrMessageID     int
}

var cfg Config
var users_info = make(map[int64]user_data)
var users_info_mu sync.Mutex
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
		bot.WithCallbackQueryDataHandler(JournalCallback, bot.MatchTypePrefix, journalcallbackHandler),
		bot.WithCallbackQueryDataHandler(FileCallback, bot.MatchTypePrefix, filecallbackHandler),
	}

	b, err := bot.New(cfg.ApiToken, opts...)
	if err != nil {
		panic(err)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, CommandHelp, bot.MatchTypeExact, helpHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, CommandStop, bot.MatchTypeExact, stopHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, CommandStatus, bot.MatchTypeExact, statusHandler)

	commands := []models.BotCommand{
		{
			Command:     CommandHelp,
			Description: "Show bot help",
		},
		{
			Command:     CommandStatus,
			Description: "Number of events from the boot.",
		},
		{
			Command:     CommandStop,
			Description: "Stop the bot",
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

	default_kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "write", CallbackData: JournalAdd},
			},
			{
				{Text: "append", CallbackData: JournalAppend},
			},
		},
	}

	message, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "How do you want to update journal?:",
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
			ChatID:    update.Message.Chat.ID,
		},
		ReplyMarkup: default_kb,
	})

	if err != nil {
		fmt.Println("Error sending msg")
	}

	users_info_mu.Lock()
	users_info[update.Message.Chat.ID] = user_data{
		Content:           update.Message.Text,
		File:              journal_filename(update.Message.Date),
		OriginalMessageID: update.Message.ID,
		CurrMessageID:     message.ID,
	}
	users_info_mu.Unlock()

}

func checkCallback(ctx context.Context, b *bot.Bot, update *models.Update) bool {
	ID := update.CallbackQuery.From.ID

	if !isUser(ID) {
		return false
	}
	if users_info[ID].Content == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: ID,
			Text:   "Action over msg outside memory",
		})
		return false
	}

	if users_info[ID].CurrMessageID != int(update.CallbackQuery.Message.Message.ID) {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: ID,
			Text:   "Action over old msg",
		})
		return false
	}
	return true
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return false
	}
	return true
}

func askFile(path string, ctx context.Context, b *bot.Bot,
	update *models.Update, chatID int64, msgID int) (int, error) {
	file_kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "replace", CallbackData: FileReplace},
				{Text: "overwrite", CallbackData: FileAppend},
			},
			{
				{Text: "read", CallbackData: FileRead},
				{Text: "cancel", CallbackData: FileCancel},
			},
		},
	}
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "What to do with the file: " + path,
		ReplyParameters: &models.ReplyParameters{
			MessageID: msgID,
			ChatID:    chatID,
		},
		ReplyMarkup: file_kb,
	})
	var ID int
	if err == nil {
		ID = msg.ID
	} else {
		ID = 0
	}
	return ID, err
}

func journalcallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !checkCallback(ctx, b, update) {
		return
	}
	path := journal_filename(update.CallbackQuery.Message.Message.Date)
	var f *os.File
	var err error

	switch update.CallbackQuery.Data {
	case JournalAdd:
		if FileExists(path) {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.CallbackQuery.From.ID,
				Text:   "File already exists.",
				ReplyParameters: &models.ReplyParameters{
					MessageID: users_info[update.CallbackQuery.From.ID].OriginalMessageID,
					ChatID:    update.CallbackQuery.From.ID,
				},
			})
			var ID int
			ID, err = askFile(path, ctx, b, update,
				update.CallbackQuery.From.ID, users_info[update.CallbackQuery.From.ID].OriginalMessageID)

			if err == nil {
				users_info_mu.Lock()
				udata := users_info[update.CallbackQuery.From.ID]
				udata.File = path
				udata.CurrMessageID = ID
				users_info[update.CallbackQuery.From.ID] = udata
				users_info_mu.Unlock()
			}

			return
		}
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	case JournalAppend:
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	}
	if err != nil {
		report_error(ctx, b, update.Message.Chat.ID, err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(users_info[update.CallbackQuery.From.ID].Content); err != nil {
		report_error(ctx, b, update.CallbackQuery.From.ID, err)
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.CallbackQuery.From.ID,
		Text:   "Text added to journal",
		ReplyParameters: &models.ReplyParameters{
			MessageID: users_info[update.CallbackQuery.From.ID].OriginalMessageID,
			ChatID:    update.CallbackQuery.From.ID,
		},
	})

	delete(users_info, update.CallbackQuery.From.ID)
}

func filecallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.CallbackQuery.From.ID) {
		return
	}
	if !checkCallback(ctx, b, update) {
		return
	}

	path := users_info[update.CallbackQuery.From.ID].File
	content := users_info[update.CallbackQuery.From.ID].Content

	switch update.CallbackQuery.Data {
	case FileCancel:
		delete(users_info, update.CallbackQuery.From.ID)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   "Operation cancelled",
		})
		delete(users_info, update.CallbackQuery.From.ID)
	case FileRead:
		data, err := os.ReadFile(path)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   string(data),
		})
	case FileReplace:
		f, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   "File replaced successfully",
		})
		delete(users_info, update.CallbackQuery.From.ID)
	case FileAppend:
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString("\n\n" + content); err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.From.ID,
			Text:   "Content added to file",
		})
		delete(users_info, update.CallbackQuery.From.ID)
	}

}

func report_error(ctx context.Context, b *bot.Bot, id int64, err error) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: id,
		Text:   "*Error*:\n" + err.Error(),
	})
	fmt.Println("*Error*:\n", err)
}

func helpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Sent a message and you will be prompted with options.",
	})
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
