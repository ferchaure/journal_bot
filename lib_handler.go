package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"os"
	"os/signal"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var events_count int64 = 0
var users_info = make(map[int64]user_data)
var users_info_mu sync.Mutex

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

type BotContext struct {
	Ctx       context.Context
	Bot       *bot.Bot
	deferFunc func()
}

func StartBot(bc BotContext) {
	bc.Bot.Start(bc.Ctx)
}

func (bc *BotContext) ExecuteDefer() {
	bc.deferFunc()
}

func GetNValidations() int64 {
	return atomic.LoadInt64(&events_count)
}

func isUser(val int64) bool {
	atomic.AddInt64(&events_count, 1)
	for _, v := range Cfg.Users {
		if v == val {
			return true
		}
	}
	return false
}

func ConfigBot() BotContext {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
		bot.WithCallbackQueryDataHandler(JournalCallback, bot.MatchTypePrefix, journalcallbackHandler),
		bot.WithCallbackQueryDataHandler(FileCallback, bot.MatchTypePrefix, filecallbackHandler),
	}

	b, err := bot.New(Cfg.ApiToken, opts...)
	if err != nil {
		defer cancel()
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

	for _, v := range Cfg.Users {
		scope := models.BotCommandScopeChat{ChatID: v}
		b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: commands,
			Scope:    &scope,
		})
	}

	return BotContext{
		Ctx:       ctx,
		Bot:       b,
		deferFunc: cancel,
	}
}

func report_error(ctx context.Context, b *bot.Bot, id int64, err error) {
	SendMsg(ctx, b, "*Error*:\n"+err.Error(), id)
	fmt.Println("*Error*:\n", err)
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
		SendMsg(ctx, b, "Action over msg outside memory", ID)
		return false
	}

	if users_info[ID].CurrMessageID != int(update.CallbackQuery.Message.Message.ID) {
		SendMsg(ctx, b, "Action over old msg", ID)
		return false
	}
	return true
}

func askFile(path string, ctx context.Context, b *bot.Bot, chatID int64, msgID int) (int, error) {
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
			ID, err = askFile(path, ctx, b,
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
		f, err = os.Create(path)
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
		SendMsg(ctx, b, "Operation cancelled", update.CallbackQuery.From.ID)
		delete(users_info, update.CallbackQuery.From.ID)
	case FileRead:
		data, err := os.ReadFile(path)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		SendMsg(ctx, b, string(data), update.CallbackQuery.From.ID)

	case FileReplace:
		f, err := os.Create(path)
		if err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			report_error(ctx, b, update.CallbackQuery.From.ID, err)
			return
		}
		SendMsg(ctx, b, "File replaced successfully", update.CallbackQuery.From.ID)
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
		SendMsg(ctx, b, "Content added to file", update.CallbackQuery.From.ID)
		delete(users_info, update.CallbackQuery.From.ID)
	}

}

func helpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	SendMsg(ctx, b, "Sent a message and you will be prompted with options.", update.Message.Chat.ID)
}
func statusHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	SendMsg(ctx, b, fmt.Sprintf("Events since boot: %d", GetNValidations()), update.Message.Chat.ID)
}

func stopHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !isUser(update.Message.Chat.ID) {
		return
	}
	SendMsg(ctx, b, "Stopping bot...", update.Message.Chat.ID)
	os.Exit(0)
}

func SendMsg(ctx context.Context, b *bot.Bot, Text string, ChatID any) int {
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: ChatID,
		Text:   Text,
	})
	if err != nil {
		fmt.Println("Error sending message")
		return 0
	}
	return msg.ID
}
