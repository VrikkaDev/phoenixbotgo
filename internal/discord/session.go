package discord

import (
	"fmt"
	"phoenixbot/internal/config"
	"time"

	"github.com/bwmarrin/discordgo"
)

type ClearMessagesOnChannelOptions struct {
	Blacklist []string // User ids to exclude
	Whitelist []string // User ids to include
	Before    string   // Message id to fetch messages before
	After     string   // Message id to fetch messages after
	Limit     int
}

type MessageData struct {
	Content string     `json:"Content,omitempty"`
	Embed   *EmbedData `json:"Embed,omitempty"`
}

type EmbedData struct {
	Title       string `json:"Title,omitempty"`
	Description string `json:"Description,omitempty"`
	URL         string `json:"Url,omitempty"`
	Color       string `json:"Color,omitempty"`
	Footer      Footer `json:"Footer,omitempty"`
	Image       string `json:"Image,omitempty"`
	Thumbnail   string `json:"Thumbnail,omitempty"`
}

type Footer struct {
	Text    string `json:"Text,omitempty"`
	IconURL string `json:"Icon_url,omitempty"`
}

func SendReplyMessageTimed(session *discordgo.Session, channelID, messageID, content string, timeout time.Duration) error {
	msg, err := session.ChannelMessageSendReply(channelID, content, &discordgo.MessageReference{
		MessageID: messageID,
		ChannelID: channelID,
		GuildID:   "",
	})
	if err != nil {
		return err
	}

	time.AfterFunc(timeout, func() {
		err := session.ChannelMessageDelete(channelID, msg.ID)
		if err != nil {
			config.Logger.Warnf("Failed to delete message %s: %v", msg.ID, err)
		}
	})

	return nil
}

func SendInteractionResponse(session *discordgo.Session, interaction *discordgo.Interaction, msg *discordgo.MessageSend) error {
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg.Content,
			Embeds:  []*discordgo.MessageEmbed{msg.Embed},
		},
	}

	return session.InteractionRespond(interaction, response)
}

func GetUserVoiceState(s *discordgo.Session, guildID, userID string) *discordgo.VoiceState {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		config.Logger.Errorln("Failed to get guild:", err)
		return nil
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return vs
		}
	}
	return nil
}

func ClearMessagesOnChannel(session *discordgo.Session, channelID string, options *ClearMessagesOnChannelOptions) error {
	if options == nil {
		options = &ClearMessagesOnChannelOptions{}
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}

	messages, err := session.ChannelMessages(channelID, limit, options.Before, options.After, "")
	if err != nil {
		return err
	}

	blacklistMap := make(map[string]struct{})
	for _, id := range options.Blacklist {
		blacklistMap[id] = struct{}{}
	}

	whitelistMap := make(map[string]struct{})
	for _, id := range options.Whitelist {
		whitelistMap[id] = struct{}{}
	}

	var messagesToDelete []string
	for _, msg := range messages {
		authorID := msg.Author.ID

		if _, blacklisted := blacklistMap[authorID]; blacklisted {
			continue
		}

		if len(whitelistMap) > 0 {
			if _, whitelisted := whitelistMap[authorID]; !whitelisted {
				continue
			}
		}

		messagesToDelete = append(messagesToDelete, msg.ID)
	}

	for i := 0; i < len(messagesToDelete); i += 100 {
		end := i + 100
		if end > len(messagesToDelete) {
			end = len(messagesToDelete)
		}

		if err := session.ChannelMessagesBulkDelete(channelID, messagesToDelete[i:end]); err != nil {
			config.Logger.Infoln("Failed to delete messages in channel %s: %v", channelID, err)
		}
	}

	return nil
}

func CreateMessageSend(message MessageData) (*discordgo.MessageSend, error) {
	mess := &discordgo.MessageSend{}

	embed, err := CreateEmbed(message.Embed)
	if err != nil {
		return nil, err
	}

	mess.Embed = embed
	mess.Content = message.Content

	return mess, nil
}

func CreateEmbed(message *EmbedData) (*discordgo.MessageEmbed, error) {

	if message == nil {
		return nil, nil
	}

	embed := &discordgo.MessageEmbed{}

	if message.Title != "" {
		embed.Title = message.Title
	}
	if message.Description != "" {
		embed.Description = message.Description
	}
	if message.URL != "" {
		embed.URL = message.URL
	}
	if message.Color != "" {
		embed.Color = parseHexColor(message.Color)
	}
	if message.Footer.Text != "" || message.Footer.IconURL != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text:    message.Footer.Text,
			IconURL: message.Footer.IconURL,
		}
	}
	if message.Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: message.Thumbnail}
	}
	if message.Image != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: message.Image}
	}

	return embed, nil
}

func parseHexColor(color string) int {
	var parsedColor int
	_, err := fmt.Sscanf(color, "0x%x", &parsedColor)
	if err != nil {
		return 0xFFFFFF // Default to white if parsing fails
	}
	return parsedColor
}
