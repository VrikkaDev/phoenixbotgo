package util

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

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
