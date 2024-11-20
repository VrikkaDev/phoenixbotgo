package bot

import (
	"os"
	"os/signal"
	"phoenixbot/internal/cog"
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"
	"syscall"
)

func Run() {
	config.Load()

	discord.Init()
	initCogs()

	defer discord.Session.Close()

	config.Logger.Infoln("Bot is running.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func initCogs() {

	if discord.Session == nil {
		config.Logger.Panic("Init cogs before initializing discord session")
	}

	cogList := []cog.Cog{
		&cog.TicketCog{},
	}

	for _, c := range cogList {
		config.Logger.Infoln("Loading cog:", c.Name())
		err := c.Init(discord.Session)
		if err != nil {
			config.Logger.Fatal("Error initializing cog:", c.Name(), err)
		}
	}
}
