package bot

import (
	"fmt"
	"os"
	"os/signal"
	"phoenixbot/internal/cog"
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"
	"sync"
	"syscall"
)

func Run() {
	config.Load()

	discord.Init()
	initCogs()
	discord.InitConnection()

	defer discord.Session.Close()

	config.Logger.Infoln("Bot is running.")
	fmt.Println("Bot is running")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func initCogs() {

	if discord.Session == nil {
		config.Logger.Panic("Tried to init cogs before initializing discord session")
	}

	cogList := []cog.Cog{
		&cog.TicketCog{
			ConfigName:  "ticket.json5",
			Session:     discord.Session,
			TicketUsers: sync.Map{},
		},
		&cog.CommandCog{
			ConfigName: "command.json5",
			Session:    discord.Session,
		},
	}

	config.Logger.Infoln("Loading cogs ...")
	for _, c := range cogList {
		err := c.Init()
		if err != nil {
			config.Logger.Fatal("Error initializing cog:", c.Name(), err)
		}
	}
}
