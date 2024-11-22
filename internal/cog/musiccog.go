package cog

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"phoenixbot/internal/config"
	"phoenixbot/internal/util"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
)

type Song struct {
	Title    string
	URL      string
	Duration string
}

type MusicConfig struct {
	Enabled        bool   `json:"Enabled"`
	Music_channel  string `json:"Music_channel"`
	Max_queue_size int    `json:"Max_queue_size"`
	Embed_colors   struct {
		Playing string `json:"Playing"`
		Paused  string `json:"Paused"`
		Error   string `json:"Error"`
	} `json:"Embed_colors"`
}

type MusicCog struct {
	Cog
	Session    *discordgo.Session
	ConfigName string

	Queue            []Song
	QueueMutex       sync.RWMutex
	CurrentlyPlaying *Song
	IsPlaying        bool

	MessageId string // Id of the music bot message

	VoiceConnection *discordgo.VoiceConnection

	Config  *MusicConfig
	Youtube *youtube.Client
}

func (m *MusicCog) Name() string {
	return "MusicCog"
}

func (m *MusicCog) Init() error {
	var musicConfig MusicConfig
	if err := config.LoadConfig(m.ConfigName, &musicConfig); err != nil {
		return nil
	}
	m.Config = &musicConfig

	if !musicConfig.Enabled {
		config.Logger.Infoln("Music feature disabled in configs")
		return nil
	}

	util.ClearMessagesOnChannel(m.Session, m.Config.Music_channel, nil)

	m.QueueMutex = sync.RWMutex{}

	m.Queue = make([]Song, 0)
	m.Youtube = &youtube.Client{}

	m.Session.AddHandler(m.handleMessage)
	m.Session.AddHandler(m.handleInteraction)

	m.Session.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
		m.updateMusicEmbed(s)
	})

	return nil
}

func (m *MusicCog) handleMessage(s *discordgo.Session, msg *discordgo.MessageCreate) {
	if msg.Author.Bot || msg.ChannelID != m.Config.Music_channel {
		return
	}

	channel, err := s.Channel(msg.ChannelID)
	if err != nil {
		config.Logger.Errorln("Failed to get channel:", err)
		return
	}

	guildID := channel.GuildID
	voiceState := util.GetUserVoiceState(s, guildID, msg.Author.ID)
	if voiceState == nil {
		_, err := s.ChannelMessageSendReply(msg.ChannelID, "You must be in a voice channel to add songs!", &discordgo.MessageReference{
			MessageID: msg.ID,
		})
		if err != nil {
			config.Logger.Errorln("Failed to send reply:", err)
		}
		return
	}

	err = m.joinVoiceChannelIfNeeded(guildID, voiceState.ChannelID)
	if err != nil {
		_, err := s.ChannelMessageSendReply(msg.ChannelID, fmt.Sprintf("Failed to join voice channel: %v", err), &discordgo.MessageReference{
			MessageID: msg.ID,
		})
		if err != nil {
			config.Logger.Errorln("Failed to send reply:", err)
		}
		return
	}

	video, err := m.fetchYouTubeVideo(msg.Content)
	if err != nil {
		_, err := s.ChannelMessageSendReply(msg.ChannelID, "Failed to fetch video. Please ensure the URL is valid.", &discordgo.MessageReference{
			MessageID: msg.ID,
		})
		if err != nil {
			config.Logger.Errorln("Failed to send reply:", err)
		}
		return
	}

	m.QueueMutex.Lock()
	defer m.QueueMutex.Unlock()

	song := Song{
		Title:    video.Title,
		URL:      msg.Content,
		Duration: fmt.Sprintf("%02d:%02d", video.Duration/time.Minute, (video.Duration%time.Minute)/time.Second),
	}
	m.Queue = append(m.Queue, song)
	if !m.IsPlaying {
		m.startQueueWorker(s)
	}

	s.ChannelMessageDelete(m.Config.Music_channel, msg.ID)
}

func (m *MusicCog) fetchYouTubeVideo(url string) (*youtube.Video, error) {
	video, err := m.Youtube.GetVideo(url)
	if err != nil {
		return nil, err
	}
	return video, nil
}

func (m *MusicCog) joinVoiceChannelIfNeeded(guildID, channelID string) error {
	if m.VoiceConnection != nil && m.VoiceConnection.ChannelID == channelID {
		return nil
	}

	if m.VoiceConnection != nil {
		m.VoiceConnection.Disconnect()
		time.Sleep(100 * time.Millisecond)
	}

	vc, err := m.Session.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %v", err)
	}

	vc.OpusSend = make(chan []byte, 2048)
	m.VoiceConnection = vc
	return nil
}

func (m *MusicCog) handleInteraction(s *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction.Type != discordgo.InteractionMessageComponent {
		return
	}

	m.QueueMutex.Lock()
	defer m.QueueMutex.Unlock()

	switch interaction.MessageComponentData().CustomID {
	case "phoenix_music_play":
		if !m.IsPlaying {
			m.resumePlayback(s, interaction)
		}
	case "phoenix_music_pause":
		m.pausePlayback(s, interaction)
	case "phoenix_music_skip":
		m.skipSong(s, interaction)
	case "phoenix_music_disconnect":
		m.disconnectFromVoice(s)
	}
}

func (m *MusicCog) startQueueWorker(s *discordgo.Session) {
	go func() {
		for {
			m.QueueMutex.Lock()
			if len(m.Queue) == 0 {
				m.IsPlaying = false
				m.CurrentlyPlaying = nil
				m.QueueMutex.Unlock()
				m.updateMusicEmbed(s)
				break
			}

			m.CurrentlyPlaying = &m.Queue[0]
			m.Queue = m.Queue[1:]
			m.IsPlaying = true
			m.QueueMutex.Unlock()

			m.updateMusicEmbed(s)

			err := m.streamCurrentSong(s)
			if err != nil {
				fmt.Printf("Error streaming song: %v\n", err)
			}
		}
	}()
}

func (m *MusicCog) streamCurrentSong(s *discordgo.Session) error {
	if m.CurrentlyPlaying == nil {
		return nil
	}

	streamIoReadCloser, err := m.getYouTubeStreamURL(m.CurrentlyPlaying.URL)
	if err != nil {
		return fmt.Errorf("failed to get stream URL: %v", err)
	}
	defer streamIoReadCloser.Close()

	if m.VoiceConnection == nil {
		return fmt.Errorf("no voice connection available")
	}

	return m.streamAudio(m.VoiceConnection, streamIoReadCloser)
}

func (m *MusicCog) getYouTubeStreamURL(videoURL string) (io.ReadCloser, error) {
	cmd := exec.Command("yt-dlp", "-f", "bestaudio", "--quiet", "-o", "-", videoURL)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp stdout pipe error: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %v", err)
	}

	return stdout, nil
}

func (m *MusicCog) streamAudio(vc *discordgo.VoiceConnection, stream io.ReadCloser) error {
	pipeReader, pipeWriter := io.Pipe()

	defer pipeReader.Close()
	defer pipeWriter.Close()
	defer stream.Close()

	if err := vc.Speaking(true); err != nil {
		return fmt.Errorf("failed to start speaking: %v", err)
	}
	defer vc.Speaking(false)

	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "opus", "-ar", "48000", "-ac", "2", "-frame_duration", "20", "-application", "lowdelay", "pipe:1")
	cmd.Stdin = pipeReader
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe error: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stream.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Println("Error reading stream:", err)
				}
				break
			}
			if _, err := pipeWriter.Write(buffer[:n]); err != nil {
				log.Println("Error writing to ffmpeg:", err)
				break
			}
		}
	}()

	buffer := make([]byte, 960*8)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := stdout.Read(buffer)
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return fmt.Errorf("error reading ffmpeg output: %v", err)
			}
			vc.OpusSend <- buffer[:n]
		}
	}
}

func (m *MusicCog) pausePlayback(s *discordgo.Session, _ *discordgo.InteractionCreate) {
	m.IsPlaying = false
	m.updateMusicEmbed(s)
}

func (m *MusicCog) resumePlayback(s *discordgo.Session, _ *discordgo.InteractionCreate) {
	if m.CurrentlyPlaying != nil && !m.IsPlaying {
		m.IsPlaying = true
		m.startQueueWorker(s)
		m.updateMusicEmbed(s)
		config.Logger.Infoln("Resumed playback")
	}
}

func (m *MusicCog) skipSong(s *discordgo.Session, _ *discordgo.InteractionCreate) {
	m.startQueueWorker(s)
	m.updateMusicEmbed(s)
}

func (m *MusicCog) updateMusicEmbed(s *discordgo.Session) {
	var color int
	if m.IsPlaying {
		color = 0x00FF00 // Playing
	} else {
		color = 0xFFFF00 // Paused
	}

	description := "No songs currently playing."
	if m.CurrentlyPlaying != nil {
		description = fmt.Sprintf("**Now Playing:** [%s](%s) (%s)\n\n**Queue:**\n", m.CurrentlyPlaying.Title, m.CurrentlyPlaying.URL, m.CurrentlyPlaying.Duration)
		for i, song := range m.Queue {
			description += fmt.Sprintf("%d. [%s](%s) (%s)\n", i+1, song.Title, song.URL, song.Duration)
			if i >= 4 {
				description += "...and more\n"
				break
			}
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Now Playing",
		Description: description,
		Color:       color,
	}

	buttons := []discordgo.MessageComponent{
		discordgo.Button{Label: "▶️", CustomID: "phoenix_music_play", Style: discordgo.SuccessButton},
		discordgo.Button{Label: "⏸️", CustomID: "phoenix_music_pause", Style: discordgo.SecondaryButton},
		discordgo.Button{Label: "⏭️", CustomID: "phoenix_music_skip", Style: discordgo.SecondaryButton},
		discordgo.Button{Label: "Disconnect", CustomID: "phoenix_music_disconnect", Style: discordgo.DangerButton},
	}

	if m.MessageId == "" {
		msg, err := s.ChannelMessageSendComplex(m.Config.Music_channel, &discordgo.MessageSend{
			Embed: embed,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: buttons},
			},
		})
		if err != nil {
			config.Logger.Errorln("Failed to send music embed:", err)
			return
		}
		m.MessageId = msg.ID
	} else {
		_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Embed:      embed,
			Components: &[]discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}},
			Channel:    m.Config.Music_channel,
			ID:         m.MessageId,
		})
		if err != nil {
			config.Logger.Errorln("Failed to update music embed:", err)
		}
	}
}

func (m *MusicCog) disconnectFromVoice(s *discordgo.Session) {
	if m.VoiceConnection != nil {
		m.VoiceConnection.Disconnect()
		m.VoiceConnection = nil
		m.IsPlaying = false
		m.CurrentlyPlaying = nil
		m.Queue = nil
		m.updateMusicEmbed(s)
		fmt.Println()
		config.Logger.Infoln("Disconnected from the voice channel")
		return
	}
	config.Logger.Warnln("Tried to disconnect while not in voice")
}
