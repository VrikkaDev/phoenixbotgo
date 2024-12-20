package discord

import (
	"phoenixbot/internal/config"

	"github.com/bwmarrin/discordgo"
)

var Session *discordgo.Session

func Init() {
	var err error
	Session, err = discordgo.New("Bot " + config.Configuration.DiscordToken)
	if err != nil {
		config.Logger.Error("Failed creating discordgo session ", err)
	}
	Session.Identify.Intents = discordgo.IntentsAll
}

func InitConnection() {
	if err := Session.Open(); err != nil {
		config.Logger.Error("failed to create websocket connection to discord", "error", err)
		return
	}
}
