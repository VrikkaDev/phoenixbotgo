package util

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

type MessageData struct {
	Content string    `json:"content,omitempty"`
	Embed   EmbedData `json:"embed,omitempty"`
}

type EmbedData struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Color       string `json:"color,omitempty"`
	Footer      Footer `json:"footer,omitempty"`
	Image       string `json:"image,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

type Footer struct {
	Text    string `json:"text,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
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

func CreateEmbed(message EmbedData) (*discordgo.MessageEmbed, error) {

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
