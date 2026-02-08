package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/kkdai/youtube/v2"
)

// Link types (ID=sRlsQJOrABY):
// https://www.youtube.com/watch?v=sRlsQJOrABY
// https://youtu.be/sRlsQJOrABY?si=wySPGaIQ8ZDKHWPb
// !! NOT SUPPORTED https://www.youtube.com/watch?v=NncelKQ6Hvw&list=RDpULkm-3b_1M&index=5

// Parses the link provided and returns the video ID. Used in Download().
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
func Download(videoID string, proxyURL url.URL) (string, string, error) {
	//Creates custom HTTP Client with proxy and ensure connection is closed after download
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&proxyURL),
		},
	}
	defer httpClient.CloseIdleConnections()

	client := youtube.Client{
		HTTPClient: httpClient,
	}

	video, err := client.GetVideo(videoID)
	if err != nil {
		return "", "Error fetching video metadata.", err
	}

	// Returns before download attempt if video duration is greater than 30 minutes
	if video.Duration.Minutes() > 30 {
		return "", "Video exceeds 30 minutes, please reattempt with a shorter video duration.", fmt.Errorf("Video too long - Duration: %f", video.Duration.Minutes())
	}

	log.Printf("Downloading: %s\n", video.Title)

	formats := video.Formats.WithAudioChannels()
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		return "", "Error downloading: internal error", err
	}

	// Creates {videoID}.mp4 in root directory
	fileName := fmt.Sprintf("%s.mp4", videoID)
	file, err := os.Create(fileName)
	if err != nil {
		return "", "Error downloading: internal error", err
	}
	defer file.Close()

	// Copies data from stream into video.mp4
	_, err = io.Copy(file, stream)
	if err != nil {
		return "", "Error downloading: internal error", err
	}

	return video.Title, fmt.Sprintf("Successfully downloaded video titled: '%s' - Attempting to convert", video.Title), nil
}

// Converts the MP4 to MP3 via ffmpeg.
func Convert(videoID string) error {
	mp4File := fmt.Sprintf("%s.mp4", videoID)
	mp3File := fmt.Sprintf("%s.mp3", videoID)
	cmd := exec.Command("ffmpeg", "-i", mp4File, mp3File)
	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

// Deletes both MP4 and MP3 from files.
func Cleanup(videoID string) error {
	mp4File := fmt.Sprintf("%s.mp4", videoID)
	mp3File := fmt.Sprintf("%s.mp3", videoID)
	err := os.Remove(mp4File)
	if err != nil {
		return err
	}
	log.Printf("Successfully deleted %s", mp4File)

	err = os.Remove(mp3File)
	if err != nil {
		return err
	}
	log.Printf("Successfully deleted %s", mp3File)

	return nil
}

// Uploads the MP3 to S3 bucket.
func UploadMP3(s3Client *s3.Client, s3Bucket string, fileName string) (string, error) {
	bucketKey := fmt.Sprintf("mp3s/%s", fileName)

	file, err := os.Open(fileName)
	if err != nil {
		return "", fmt.Errorf("Failed to open file: %s", err)
	}
	defer file.Close()

	uploader := manager.NewUploader(s3Client)

	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(bucketKey),
		Body:   file,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to upload MP3 to S3 bucket: %s", err)
	}

	log.Print("Successfully uploaded file to S3 bucket.")

	return "", nil
}

// Checks if the video ID already exists in the S3 bucket, returns true if so unless older than 12 hours.
func CheckExistingMP3(s3Client *s3.Client, s3Bucket string, fileName string) (bool, error) {
	bucketKey := fmt.Sprintf("mp3s/%s", fileName)

	headData, err := s3Client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(bucketKey),
	})
	if err != nil {
		// Checks if error type is of NoSuchKey/NotFound. If not, error is return and bot returns.
		var noSuchKey *types.NoSuchKey
		var notFound *types.NotFound
		if errors.As(err,&notFound) || errors.As(err, &noSuchKey) {
			log.Printf("MP3 doesn't exist in S3 bucket. Continuing with download.")
			return false, nil
		}
		return false, err
	}

	uploadTime := *headData.LastModified
	// Reuses file if uploaded in the last 12 hours
	if headData.LastModified != nil {
		if time.Since(uploadTime) < 12*time.Hour {
			log.Printf("Found existing MP3 in S3 bucket; reusing - File: %s Age: %.1f", fileName, time.Since(uploadTime).Hours())
			return true, nil
		}
	}

	// Re uploads file if it's older than 12 hours
	log.Printf("Found existing MP3 but is stale; reuploading - File: %s Age: %.1f", fileName, time.Since(uploadTime).Hours())
	return false, nil
}

// Generates a signed URL for the uploaded MP3.
func GenerateSignedURL(videoID string, signer *sign.URLSigner) (string, error) {
	rawURL := fmt.Sprintf("https://cdn.washedwubzi.com/mp3s/%s.mp3", videoID)
	expires := time.Now().Add(30 * time.Minute)

	signedURL, err := signer.Sign(rawURL, expires)
	if err != nil {
		return "", fmt.Errorf("Failed to created signed URL: %s", err)
	}

	return signedURL, nil
}
