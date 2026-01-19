# Muto - YouTube to MP3 Discord bot

Takes a YouTube share link via Discord slash command and serves
back an MP3.

## Requirements
- **Go 1.21+**
- **FFmpeg** - Used for MP4 to MP3 conversion. Download and add to PATH.
- **Discord Bot Token** - Create a bot at [Discord Developer Portal](https://discord.com/developers/applications)
  - Enable "Message Content Intent" in Bot settings
  - Invite bot to your server with `applications.commands` scope

## Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/muto.git
   cd muto
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Create a `.env` file in the project root:
   ```
   DISCORD_BOT_TOKEN=your_bot_token_here
   DISCORD_GUILD_ID=your_server_id_here
   ```

4. Run the bot:
   ```bash
   go run main.go
   ```

## Usage

Use the `/youtube-to-mp3` slash command in Discord with a YouTube share link.

## Motivation

I made it so my gf can use the MP3s as ringtones.

This is my first Discord bot, sorry if it's sloppy.