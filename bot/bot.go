package bot

import (
	"context"
	"fmt"
	"io"
	"strings"

	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/wvbzi/muto/proxy"
	m "github.com/wvbzi/muto/video"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	proxyPool *proxy.ProxyPool
	guildID   string
	botToken  string
	s         *discordgo.Session

	s3Bucket string
	s3Client *s3.Client
	signer   *sign.URLSigner
)

// Command option and handler implementation
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

			// Gets link option from user
			options := i.ApplicationCommandData().Options
			link := options[0].StringValue()

			// Extracts YouTube video ID from link
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

			// Validation response after successfully extracting video ID
			content = fmt.Sprintf("Successfully parsed video ID: '%s' - Attempting to download.", videoID)
			log.Print(content)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsEphemeral,
					Content: content,
				},
			})

			// Checks if the file already exists in the S3 bucket to prevent redundant downloads/uploads
			// Function returns false if video in the S3 bucket is older than 12 hours and bot proceeds with download
			fileName := fmt.Sprintf("%s.mp3", videoID)
			mp3Exists, err := m.CheckExistingMP3(s3Client, s3Bucket, fileName)
			if err != nil {
				content = "Internal error: Failed to check for existing MP3."
				log.Printf("Existing MP3 check returned different error: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			if mp3Exists {
				signedURL, err := m.GenerateSignedURL(videoID, signer)
				if err != nil {
					content = "Internal error: Failed to generate download link."
					log.Printf("Error generating signed URL: %s", err)
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: &content,
					})
					return
				}

				content = fmt.Sprintf("Successfully grabbed video MP3 from recent conversions.\n[Download here :3](%s)", signedURL)
				log.Print(content)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			// Checks if a proxy is available in the proxy pool, if not, prompts user to retry
			// If there are 3 downloads happening at the same time, the bot will return the prompt ^
			log.Print("Checking for available proxy")
			proxyURL, ok := proxyPool.ProxyAvailable()
			if !ok {
				content = "Bot is processing other downloads, please try again in a moment."
				log.Printf("Proxy not available.")
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			defer proxyPool.MakeProxyAvailable(proxyURL)

			// Attempts to download the video and defers cleanup if download/conversion is sucessful
			videoTitle, content, err := m.Download(videoID, proxyURL)
			if err != nil {
				log.Printf("Error downlading video: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			defer m.Cleanup(videoID)
			log.Print(content)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
			})

			// Converts downloaded MP4 to an MP3 with same name (videoID.mp3)
			err = m.Convert(videoID)
			if err != nil {
				content = "Error converting video to MP3. Try again."
				log.Printf("Error converting to MP3: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			// Opens MP3
			file, err := os.Open(fileName)
			if err != nil {
				content = "Internal error: Failed to open MP3 file."
				log.Printf("Failed to open MP3 file: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			defer file.Close()

			// Uploads opened MP3 to S3 bucket
			log.Print("Attempting to upload to S3 bucket")
			_, err = m.UploadMP3(s3Client, s3Bucket, fileName)
			if err != nil {
				content = "Internal error: Failed to upload MP3 file."
				log.Printf("Error uploading to S3 bucket: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			// Generates a signed URL for the uploaded MP3 through CloudFront signer
			signedURL, err := m.GenerateSignedURL(videoID, signer)
			if err != nil {
				content = "Internal error: Failed to generate download link."
				log.Printf("Error generating signed URL: %s", err)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			// Final response which returns a successful conversion message and the signed URL for download
			content = fmt.Sprintf("Successfully converted '%s' to an MP3.\n[Download here :3](%s)", videoTitle, signedURL)
			log.Print(content)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
			})
		},
	}
)

// Intiates Muto Discord bot, AWS services, and a ProxyPool.
func Init() {
	log.Println("Initializing Muto")

	// Loads .env if running local
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			log.Fatalf("Failed to load .env file: %s", err)
		}
	}

	// Saves Discord Bot Token and Guild ID from env
	botToken = os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN env variable not set")
	}
	guildID = os.Getenv("DISCORD_GUILD_ID")
	if guildID == "" {
		log.Fatal("DISCORD_GUILD_ID env variable not set")
	}

	// Initiates AWS S3 config
	err := InitS3Config()
	if err != nil {
		log.Fatalf("ERROR INITIALIZING S3 CONFIG: %s", err)
	}
	log.Println("S3 successfully initiated")

	// Initiates AWS CloudFront signer
	err = InitCloudFrontSigner()
	if err != nil {
		log.Fatalf("ERROR INITIALIZING CLOUDFRONT SIGNER: %s", err)
	}
	log.Println("CloudFront signer successfully initiated")

	// Initiates ProxyPool.
	proxyPool, err = proxy.InitProxyPool()
	if err != nil {
		log.Fatalf("ERROR INITIALIZING PROXY POOL: %s", err)
	}
	log.Println("Proxy pool successfully initiated")

	// Creates Discord session
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

	// Adds youtube-to-mp3 command to bot
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

	// Command cleanup and bot shutdown
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

// Initiates AWS S3 config.
func InitS3Config() error {
	s3Bucket = os.Getenv("S3_BUCKET_NAME")
	if s3Bucket == "" {
		log.Fatal("S3_BUCKET_NAME env variable not set")
	}

	// Auto loads AWS credentials from env / shared cred or config file / IAM role in ECS
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("Unable to load AWS config: %s", err)
	}

	s3Client = s3.NewFromConfig(cfg)

	return nil
}

// Initiates AWS CloudFront signer.
func InitCloudFrontSigner() error {
	var privKeyReader io.Reader

	keyID := os.Getenv("CLOUDFRONT_KEY_ID")
	if keyID == "" {
		log.Fatal("CLOUDFRONT_KEY_ID env variable not found")
	}

	// If CLOUDFRONT_PRIVATE_KEY_FILE isn't found, private key is extracted from ECS env
	privKeyFileName := os.Getenv("CLOUDFRONT_PRIVATE_KEY_FILE")
	if privKeyFileName == "" {
		privKeyPEM := os.Getenv("CLOUDFRONT_PRIVATE_KEY")
		if privKeyPEM == "" {
			log.Fatalf("CLOUDFRONT_PRIVATE_KEY env variable not found: Issue in AWS setup.")
		}

		privKeyReader = strings.NewReader(privKeyPEM)
	} else {
		privKeyFile, err := os.Open(privKeyFileName)
		if err != nil {
			return fmt.Errorf("Failed to open private key file: %s", err)
		}
		defer privKeyFile.Close()

		privKeyReader = privKeyFile
	}

	privKey, err := sign.LoadPEMPrivKeyPKCS8AsSigner(privKeyReader)
	if err != nil {
		return fmt.Errorf("Failed to load private key file: %s", err)
	}

	log.Print("Attemping to load signer with priv key")
	signer = sign.NewURLSigner(keyID, privKey)

	return nil
}
