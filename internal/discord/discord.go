package discord

import (
	"phoenixbot/internal/config"

	"github.com/bwmarrin/discordgo"
)

var Session *discordgo.Session

func Init() {
	var err error
	Session, err = discordgo.New("Bot-" + config.Configuration.DiscordToken)
	if err != nil {
		config.Logger.Errorln("Failed while creating discordgo session ", err)
	}
	Session.Identify.Intents = discordgo.IntentsAll
}
