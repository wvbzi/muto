package video

import (
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// Link types (ID=sRlsQJOrABY):
// https://www.youtube.com/watch?v=sRlsQJOrABY
// https://youtu.be/sRlsQJOrABY?si=wySPGaIQ8ZDKHWPb
// https://www.youtube.com/watch?v=NncelKQ6Hvw&list=RDpULkm-3b_1M&index=5

// Parses the link provided; used in Download().
func ParseLink(link string) (string, error) {
	var videoID string

	if !strings.Contains(link, "youtube.com/watch?v=") && !strings.Contains(link, "youtu.be/") {
		return videoID, errors.New("Invalid link format")
	}
	// If link is obtained via YouTube share button, the link will not contain "watch".
	if strings.Contains(link, "watch") {
		_, videoID, _ := strings.Cut(link, "=")
		return videoID, nil
	} else {
		_, s, _ := strings.Cut(link, "youtu.be/")
		videoID, _, _ = strings.Cut(s, "?")
		return videoID, nil
	}
}

// Downloads the YouTube video.
func Download(videoID string) (string, error) {
	client := youtube.Client{}

	video, err := client.GetVideo(videoID)
	if err != nil {
		return "", err
	}

	title := video.Title
	log.Printf("Downloading: %s\n", video.Title)

	formats := video.Formats.WithAudioChannels()
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		return "", err
	}

	// Creates video.mp4 in root directory	
	file, err := os.Create("video.mp4")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Copies data from stream into video.mp4
	_, err = io.Copy(file, stream)
	if err != nil {
		return "", err
	}

	return title, nil
}

// Converts the MP4 to MP3 via ffmpeg.
func Convert() error {
	cmd := exec.Command("ffmpeg", "-i", "video.mp4", "audio.mp3")
	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

// Deletes both MP4 and MP3 from files.
func Cleanup() error {
	err := os.Remove("video.mp4")
	if err != nil {
		return err
	}
	log.Print("Successfully deleted video.mp4")

	err = os.Remove("audio.mp3")
	if err != nil {
		return err
	}
	log.Print("Successfully deleted audio.mp4")

	return nil
}