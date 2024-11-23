package music

import (
	"fmt"
	"io"
	"os/exec"
)

func DecodeAudioToPCM(input io.Reader, pcmChan chan<- []int16) error {
	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	cmd.Stdin = input

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	buf := make([]byte, 48000*4)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			samples := make([]int16, n/2)
			for i := 0; i < len(samples); i++ {
				samples[i] = int16(buf[2*i]) | int16(buf[2*i+1])<<8
			}
			pcmChan <- samples
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read from ffmpeg stdout: %v", err)
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg process failed: %v", err)
	}
	return nil
}
