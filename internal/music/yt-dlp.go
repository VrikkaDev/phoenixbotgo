package music

import (
	"io"
	"os/exec"
	"phoenixbot/internal/util"
)

func GetYouTubeStream(videoURL string) (io.ReadCloser, error) {
	cmd := exec.Command("yt-dlp", "-f", "bestaudio", "-o", "-", videoURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return stdout, nil
}

func FindYouTubeVideo(videoName string) (string, error) {
	cmd := exec.Command("yt-dlp", "ytsearch1:"+"\""+videoName+"\"", "--get-id")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	b, err := io.ReadAll(stdout)
	if err != nil {
		return "", err
	}
	s := string(b)
	return util.YoutubeIdToUrl(s), nil
}
