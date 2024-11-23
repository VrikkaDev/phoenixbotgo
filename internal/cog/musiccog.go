package cog

import (
	"fmt"
	"log"
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"
	"phoenixbot/internal/music"
	"phoenixbot/internal/util"
	"sync"
	"time"

	"github.com/bwmarrin/dgvoice"
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

	MessageId       string
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
		return err
	}
	m.Config = &musicConfig

	if !musicConfig.Enabled {
		config.Logger.Infoln("Music feature disabled in configs")
		return nil
	}

	discord.ClearMessagesOnChannel(m.Session, m.Config.Music_channel, nil)

	m.Queue = make([]Song, 0)
	m.Youtube = &youtube.Client{}

	m.Session.AddHandler(m.handleMessage)
	m.Session.AddHandler(m.handleInteraction)

	m.Session.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
		m.updateMusicEmbed(s)
	})

	return nil
}

func (m *MusicCog) getYoutubeVideo(s *discordgo.Session, msg *discordgo.MessageCreate) (*youtube.Video, error) {

	video, err := m.fetchYouTubeVideo(msg.Content)
	if err == nil {
		return video, nil
	}

	newurl, err := music.FindYouTubeVideo(msg.Content)
	if err == nil {
		video, err = m.fetchYouTubeVideo(newurl)
		if err == nil {
			return video, nil
		}
	}

	return nil, fmt.Errorf("couldnt find youtube video of that name or url")
}

func (m *MusicCog) handleMessage(s *discordgo.Session, msg *discordgo.MessageCreate) {
	if msg.Author.Bot || msg.ChannelID != m.Config.Music_channel {
		return
	}

	defer s.ChannelMessageDelete(m.Config.Music_channel, msg.ID)

	voiceState := discord.GetUserVoiceState(s, msg.GuildID, msg.Author.ID)
	if voiceState == nil {
		discord.SendReplyMessageTimed(m.Session, msg.ChannelID, msg.ID, "You must be in a voice channel to add songs!", time.Second*2)
		return
	}

	err := m.joinVoiceChannelIfNeeded(msg.GuildID, voiceState.ChannelID)
	if err != nil {
		discord.SendReplyMessageTimed(m.Session, msg.ChannelID, msg.ID, fmt.Sprintf("Failed to join voice channel: %v", err), time.Second*2)
		return
	}

	video, err := m.getYoutubeVideo(s, msg)
	if err != nil {
		config.Logger.Warnln(err)
		discord.SendReplyMessageTimed(m.Session, msg.ChannelID, msg.ID, "Failed to fetch video. Please ensure the URL is valid.", time.Second*2)
		return
	}

	m.QueueMutex.Lock()
	defer m.QueueMutex.Unlock()

	song := Song{
		Title:    video.Title,
		URL:      util.YoutubeIdToUrl(video.ID),
		Duration: fmt.Sprintf("%02d:%02d", video.Duration/time.Minute, (video.Duration%time.Minute)/time.Second),
	}
	m.Queue = append(m.Queue, song)
	if !m.IsPlaying {
		m.startQueueWorker()
	}
}

func (m *MusicCog) fetchYouTubeVideo(url string) (*youtube.Video, error) {
	return m.Youtube.GetVideo(url)
}

func (m *MusicCog) joinVoiceChannelIfNeeded(guildID, channelID string) error {
	if m.VoiceConnection != nil && m.VoiceConnection.ChannelID == channelID {
		return nil
	}

	if m.VoiceConnection != nil {
		m.VoiceConnection.Disconnect()
		time.Sleep(100 * time.Millisecond)
	}

	vc, err := m.Session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %v", err)
	}
	m.VoiceConnection = vc
	return nil
}

func (m *MusicCog) handleInteraction(s *discordgo.Session, interaction *discordgo.InteractionCreate) {
	switch interaction.MessageComponentData().CustomID {
	case "phoenix_music_play":
		m.resumePlayback()
	case "phoenix_music_pause":
		m.pausePlayback()
	case "phoenix_music_skip":
		m.skipSong()
	case "phoenix_music_disconnect":
		m.disconnectFromVoice()
	}
	m.updateMusicEmbed(s)
}

func (m *MusicCog) startQueueWorker() {
	go func() {
		for {
			m.QueueMutex.Lock()
			if len(m.Queue) == 0 {
				m.IsPlaying = false
				m.CurrentlyPlaying = nil
				m.QueueMutex.Unlock()
				m.updateMusicEmbed(m.Session)
				break
			}

			m.CurrentlyPlaying = &m.Queue[0]
			m.Queue = m.Queue[1:]
			m.IsPlaying = true
			m.QueueMutex.Unlock()

			m.updateMusicEmbed(m.Session)
			err := m.streamCurrentSong()
			if err != nil {
				log.Println("Error streaming song:", err)
			}
		}
	}()
}

func (m *MusicCog) streamCurrentSong() error {
	if m.CurrentlyPlaying == nil || m.VoiceConnection == nil {
		return nil
	}

	stream, err := music.GetYouTubeStream(m.CurrentlyPlaying.URL)
	if err != nil {
		return fmt.Errorf("failed to get stream URL: %v", err)
	}
	defer stream.Close()

	pcmChan := make(chan []int16, 1024)

	go func() {
		err := music.DecodeAudioToPCM(stream, pcmChan)
		if err != nil {
			log.Printf("Error decoding audio: %v", err)
		}
		close(pcmChan)
	}()

	dgvoice.SendPCM(m.VoiceConnection, pcmChan)

	return nil
}

func (m *MusicCog) pausePlayback() {
	m.IsPlaying = false
}

func (m *MusicCog) resumePlayback() {
	if m.CurrentlyPlaying != nil && !m.IsPlaying {
		m.IsPlaying = true
		m.startQueueWorker()
	}
}

func (m *MusicCog) skipSong() {
	m.startQueueWorker()
}

func (m *MusicCog) disconnectFromVoice() {
	if m.VoiceConnection != nil {
		m.VoiceConnection.Disconnect()
		m.VoiceConnection = nil
		m.IsPlaying = false
		m.CurrentlyPlaying = nil
		m.Queue = nil
	}
}

func (m *MusicCog) updateMusicEmbed(s *discordgo.Session) {
	var color int
	if m.IsPlaying {
		color = 0x00FF00
	} else {
		color = 0xFFFF00
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
		if err == nil {
			m.MessageId = msg.ID
		}
	} else {
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Embed:      embed,
			ID:         m.MessageId,
			Channel:    m.Config.Music_channel,
			Components: &[]discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}},
		})
	}
}
