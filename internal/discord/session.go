package discord

import (
	"github.com/bwmarrin/discordgo"
)

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
