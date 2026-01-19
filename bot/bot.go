package bot

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	m "github.com/wvbzi/muto/video"
)

var (
	guildID  string
	botToken string
	s        *discordgo.Session
)

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "youtube-to-mp3",
			Description: "Provide a YouTube share link and the bot will return a download to the MP3.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "youtube-link",
					Description: "YouTube link option.",
					Required:    true,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"youtube-to-mp3": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var content string

			// Get link from user
			options := i.ApplicationCommandData().Options
			link := options[0].StringValue()

			videoID, err := m.ParseLink(link)
			if err != nil {
				log.Printf("Error parsing link: %s", err)
				content = "Invalid URL, please try again with a valid YouTube share link."
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   discordgo.MessageFlagsEphemeral,
						Content: content,
					},
				})
				return
			}

			content = fmt.Sprintf("Successfully parsed video ID: %s", videoID)
			log.Print(content)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsEphemeral,
					Content: content,
				},
			})

			videoTitle, err := m.Download(videoID)
			if err != nil {
				content = "Error downloading video."
				log.Printf("Error downlading video: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			defer m.Cleanup()

			content = "Sucessfully downloaded video."
			log.Print(content)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
			})

			err = m.Convert()
			if err != nil {
				content = "Error converting video to MP3. Try again."
				log.Printf("Error converting to MP3: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			fileInfo, err := os.Stat("audio.mp3")
			if err != nil {
				content = "Error checking file size."
				log.Printf("Error getting file size: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			if fileInfo.Size() >= 8000000 {
				content = fmt.Sprintf("File exceeds 8MB, unable to serve - File size: %d", fileInfo.Size())
				log.Print(content)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			file, err := os.Open("audio.mp3")
			if err != nil {
				content = "Failed to open MP3 file."
				log.Printf("Failed to open MP3 file: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			defer file.Close()

			content = fmt.Sprintf("Successfully converted '%s' to an MP3.", videoTitle)
			log.Print(content)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
				Files: []*discordgo.File{
					{
						Name:   "audio.mp3",
						Reader: file,
					},
				},
			})
		},
	}
)

func Init() {
	godotenv.Load()
	botToken = os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN env variable not set")
	}
	guildID = os.Getenv("DISCORD_GUILD_ID")

	// Creates Discord session
	var err error
	s, err = discordgo.New("Bot " + botToken)
	if err != nil {
		log.Fatalf("Invalid Bot Token: %s", err)
	}

	// Handler for mapping commands with commandHandlers
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	// Logs into bot
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %s", s.State.User.Username)
	})
	err = s.Open()
	if err != nil {
		log.Fatalf("Unable to open session: %s", err)
	}

	log.Println("Adding commands.")
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, v)
		if err != nil {
			log.Fatalf("Unable to create '%s' command: %s", v.Name, err)
		}
		registeredCommands[i] = cmd
	}
	defer s.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	log.Println("Press Ctrl+C to exit")
	<-stop

	log.Println("Removing old commands")
	existingCommands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		log.Printf("Error fetching commands: %s", err)
	} else {
		for _, cmd := range existingCommands {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
			if err != nil {
				log.Printf("Error deleting command %s: %s", cmd.Name, err)
			}
		}
	}
	log.Println("Bot shutting down.")
}