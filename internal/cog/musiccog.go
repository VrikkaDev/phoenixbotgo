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

type MusicGuildConfig struct {
	Enabled        bool   `json:"Enabled"`
	Music_channel  string `json:"Music_channel"`
	Max_queue_size int    `json:"Max_queue_size"`
	Embed_colors   struct {
		Playing string `json:"Playing"`
		Paused  string `json:"Paused"`
		Error   string `json:"Error"`
	} `json:"Embed_colors"`

	Queue            []Song
	CurrentlyPlaying *Song
	IsPlaying        bool

	MessageId       string
	VoiceConnection *discordgo.VoiceConnection
}

type MusicConfig struct {
	Guilds map[string]*MusicGuildConfig `json:"Guilds"`
}

type MusicCog struct {
	Cog
	Session    *discordgo.Session
	ConfigName string

	Config     *MusicConfig
	MusicMutex sync.RWMutex

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

	m.MusicMutex.Lock()
	glds := m.Config.Guilds
	m.MusicMutex.Unlock()

	for guild, mus := range glds {
		if !config.IsGuildEnabled(guild) {
			continue
		}
		if !mus.Enabled {
			config.Logger.Infoln("Music feature disabled in config, on server ", guild)
			continue
		}
		discord.ClearMessagesOnChannel(m.Session, mus.Music_channel, nil)

		m.Session.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
			m.updateMusicEmbed(s, guild)
		})

		mus.Queue = make([]Song, 0)

		m.Config.Guilds[guild] = mus
		m.Youtube = &youtube.Client{}
	}

	m.Session.AddHandler(m.handleMessage)
	m.Session.AddHandler(m.handleInteraction)

	return nil
}

func (m *MusicCog) getYoutubeVideo(s *discordgo.Session, msg *discordgo.MessageCreate) (*youtube.Video, error) {

	video, err := m.fetchYouTubeVideo(msg.Content)
	if err == nil {
		return video, nil
	} else {
		config.Logger.Warnln(err)
	}

	newurl, err := music.FindYouTubeVideo(msg.Content)
	if err == nil {
		video, err = m.fetchYouTubeVideo(newurl)
		if err == nil {
			return video, nil
		} else {
			return nil, err
		}
	}

	return nil, fmt.Errorf("couldnt find youtube video of that name or url")
}

func (m *MusicCog) getConfig(guildID string) *MusicGuildConfig {
	m.MusicMutex.RLock()
	defer m.MusicMutex.RUnlock()

	conf, ok := m.Config.Guilds[guildID]
	if !ok || !config.IsGuildEnabled(guildID) {
		return nil
	}

	return conf
}
func (m *MusicCog) handleMessage(s *discordgo.Session, msg *discordgo.MessageCreate) {

	conf := m.getConfig(msg.GuildID)
	if conf == nil {
		return
	}

	if msg.Author.Bot || msg.ChannelID != conf.Music_channel {
		return
	}

	defer s.ChannelMessageDelete(conf.Music_channel, msg.ID)

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

	song := Song{
		Title:    video.Title,
		URL:      util.YoutubeIdToUrl(video.ID),
		Duration: fmt.Sprintf("%02d:%02d", video.Duration/time.Minute, (video.Duration%time.Minute)/time.Second),
	}
	m.MusicMutex.Lock()

	conf.Queue = append(conf.Queue, song)

	m.MusicMutex.Unlock()
	if !conf.IsPlaying {
		m.startQueueWorker(msg.GuildID)
	}
}

func (m *MusicCog) fetchYouTubeVideo(url string) (*youtube.Video, error) {
	return m.Youtube.GetVideo(url)
}

func (m *MusicCog) joinVoiceChannelIfNeeded(guildID, channelID string) error {

	conf := m.getConfig(guildID)
	if conf == nil {
		return fmt.Errorf("no config on musiccog for guild %s", guildID)
	}

	if conf.VoiceConnection != nil {
		if conf.VoiceConnection.ChannelID == channelID {
			return nil
		}
		conf.VoiceConnection.Disconnect()
		time.Sleep(100 * time.Millisecond)
	}

	vc, err := m.Session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %v", err)
	}
	conf.VoiceConnection = vc
	return nil
}

func (m *MusicCog) handleInteraction(s *discordgo.Session, interaction *discordgo.InteractionCreate) {
	gid := interaction.GuildID
	switch interaction.MessageComponentData().CustomID {
	case "phoenix_music_play":
		m.resumePlayback(gid)
	case "phoenix_music_pause":
		m.pausePlayback(gid)
	case "phoenix_music_skip":
		m.skipSong(gid)
	case "phoenix_music_disconnect":
		m.disconnectFromVoice(gid)
	}
	m.updateMusicEmbed(s, gid)
}

func (m *MusicCog) startQueueWorker(guildID string) {

	conf := m.getConfig(guildID)
	if conf == nil {
		return
	}

	go func() {
		for {
			m.MusicMutex.Lock()
			if len(conf.Queue) == 0 {
				conf.IsPlaying = false
				conf.CurrentlyPlaying = nil
				m.MusicMutex.Unlock()
				m.updateMusicEmbed(m.Session, guildID)
				break
			}

			conf.CurrentlyPlaying = &conf.Queue[0]
			conf.Queue = conf.Queue[1:]
			conf.IsPlaying = true
			m.MusicMutex.Unlock()

			m.updateMusicEmbed(m.Session, guildID)

			err := m.streamCurrentSong(guildID)
			if err != nil {
				log.Println("Error streaming song:", err)
			}
		}
	}()
}

func (m *MusicCog) streamCurrentSong(guildID string) error {
	config.Logger.Debugln("Starting to stream current song")
	tconf := m.getConfig(guildID)
	if tconf == nil {
		return fmt.Errorf("no config on musiccog for guild %s", guildID)
	}

	conf := tconf
	m.MusicMutex.Lock()
	defer m.MusicMutex.Unlock()

	if conf.CurrentlyPlaying == nil || conf.VoiceConnection == nil {
		return nil
	}

	config.Logger.Debugln("Fetching stream for:", conf.CurrentlyPlaying.URL)
	u := conf.CurrentlyPlaying.URL
	stream, err := music.GetYouTubeStream(u)
	if err != nil {
		config.Logger.Errorln("Failed to get stream:", err)
		return err
	}
	defer stream.Close()

	config.Logger.Debugln("Stream obtained, starting to decode PCM")
	pcmChan := make(chan []int16, 1024)
	go func() {
		if err := music.DecodeAudioToPCM(stream, pcmChan); err != nil {
			config.Logger.Errorln("Error decoding audio: %v", err)
		}
		close(pcmChan)
	}()

	dgvoice.SendPCM(conf.VoiceConnection, pcmChan)

	return nil
}

func (m *MusicCog) pausePlayback(guildID string) {

	conf := m.getConfig(guildID)
	if conf == nil {
		config.Logger.Warnln("no config on musiccog for guild ", guildID)
		return
	}
	conf.IsPlaying = false
}

func (m *MusicCog) resumePlayback(guildID string) {
	conf := m.getConfig(guildID)
	if conf == nil {
		config.Logger.Warnln("no config on musiccog for guild ", guildID)
		return
	}
	if conf.CurrentlyPlaying != nil && !conf.IsPlaying {
		conf.IsPlaying = true
		m.startQueueWorker(guildID)
	}
}

func (m *MusicCog) skipSong(guildID string) {
	m.startQueueWorker(guildID)
}

func (m *MusicCog) disconnectFromVoice(guildID string) {

	conf := m.getConfig(guildID)
	if conf == nil {
		config.Logger.Warnln("no config on musiccog for guild ", guildID)
		return
	}
	m.MusicMutex.Lock()
	defer m.MusicMutex.Unlock()
	if conf.VoiceConnection != nil {
		conf.VoiceConnection.Disconnect()
		conf.VoiceConnection = nil
		conf.IsPlaying = false
		conf.CurrentlyPlaying = nil
		conf.Queue = nil
	}
}

func (m *MusicCog) updateMusicEmbed(s *discordgo.Session, guildID string) {

	conf := m.getConfig(guildID)
	if conf == nil {
		config.Logger.Warnln("no config on musiccog for guild ", guildID)
		return
	}

	var color int
	if conf.IsPlaying {
		color = 0x00FF00
	} else {
		color = 0xFFFF00
	}

	description := "No songs currently playing."
	if conf.CurrentlyPlaying != nil {
		description = fmt.Sprintf("**Now Playing:** [%s](%s) (%s)\n\n**Queue:**\n", conf.CurrentlyPlaying.Title, conf.CurrentlyPlaying.URL, conf.CurrentlyPlaying.Duration)
		for i, song := range conf.Queue {
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

	config.Logger.Debugln("messageid------", conf.MessageId)

	m.MusicMutex.Lock()
	msgID := conf.MessageId
	m.MusicMutex.Unlock()

	if msgID == "" {
		msg, err := s.ChannelMessageSendComplex(conf.Music_channel, &discordgo.MessageSend{
			Embed: embed,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: buttons},
			},
		})
		if err == nil {
			m.MusicMutex.Lock()
			conf.MessageId = msg.ID
			m.MusicMutex.Unlock()
		} else {
			config.Logger.Errorln(err)
		}
	} else {
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Embed:      embed,
			ID:         conf.MessageId,
			Channel:    conf.Music_channel,
			Components: &[]discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}},
		})
	}
}
