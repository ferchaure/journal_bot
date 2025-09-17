# Journal Bot

A Telegram bot for managing personal journal entries in Markdown format. This bot allows authorized users to easily add and append text to daily journal files through an interactive Telegram interface.

## ⚠️ Work in Progress

This project is currently under active development. While the core functionality is working, many features are not implemented and the codebase will change all the time.

## 🚀 Main Features

### Current Functionality

- **User Authentication**: Only authorized users (configured in `config.toml`) can interact with the bot
- **Interactive Journal Management**: Send any message to the bot and get prompted with action options in a reply
- **Daily Journal Files**: Automatically creates and manages journal files with date-based naming (`YYYY-MM-DD.md`)
- **Two Writing Modes**:
  - **Add**: Creates a new journal entry for the day
  - **Append**: Adds content to an existing journal entry
- **File Safety**: Prevents overwriting existing entries when using "Add" mode
- **Bot Commands**:
  - `/help` - Show bot usage instructions
  - `/status` - Display number of events since bot startup
  - `/stop` - Stop the bot (admin only)

## 🛠️ Setup

### Prerequisites

- Go 1.25.1 or later
- A Telegram Bot Token (obtain from [@BotFather](https://t.me/botfather))
- A directory for storing journal files

### Installation

1. Clone the repository:
```bash
git clone https://github.com/ferchaure/journal_bot.git
cd journal_bot
```

2. Install dependencies:
```bash
go mod tidy
```

3. Configure the bot by editing `config.toml`:
```toml
# API token from BotFather
api_token = "YOUR_BOT_TOKEN_HERE"
# List of authorized user IDs
users = [123456789, 987654321]
# Directory where journal files will be stored
journal_folder = "/path/to/your/journal/directory"
```

4. Run the bot:
```bash
go run .
```

## 📖 Usage

1. Start a conversation with your bot on Telegram
2. Send any message to the bot
3. Choose your action:
   - **"try add"**: Create a new journal entry for today
   - **"append"**: Add content to today's existing journal entry
4. The bot will confirm when your text has been added to the journal

## 🔮 Future Features

The following features I want to add:

- **Read Specific Files**: Read content from specific `.md` files by date or filename
- **File Listing**: Browse available journal files
- **File Search**: Search through journal entries for specific content
- **File Deletion**: Remove specific journal entries (with confirmation)
- **Error Handling**: A decent log of activity and actions
- **Custom Commands**: Allow users to define custom actions for specific files
- **Scheduled Actions**: Automatically notify reading tasks from notes
- **Image Attachments**: Handle and store images in journal entries
- **Easy Library Replacement**: Switch to a different Telegram bot library or implement manual API handling