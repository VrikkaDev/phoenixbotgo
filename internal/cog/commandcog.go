package cog

import (
	"phoenixbot/internal/config"
	"phoenixbot/internal/util"

	"github.com/bwmarrin/discordgo"
)

type CommandConfig struct {
	Messages struct {
		TicketCreated     string `json:"TicketCreated"`
		NoPermission      string `json:"NoPermission"`
		CloseTicketPrompt string `json:"CloseTicketPrompt"`
		AlreadyHasTicket  string `json:"AlreadyHasTicket"`

		TicketChannelMessage util.MessageData `json:"TicketChannelMessage"`
		TicketCreateMessage  util.MessageData `json:"TicketCreateMessage"`
	} `json:"Messages"`

	Channel  int            `json:"Channel"`
	AddRoles map[string]int `json:"AddRoles"`
	Enabled  bool           `json:"Enabled"`
}

type CommandCog struct {
	Cog

	ConfigName string

	Session *discordgo.Session
	Config  *CommandConfig
}

func (m *CommandCog) Name() string {
	return "CommandCog"
}

func (m *CommandCog) Init() error {

	var commandConfig CommandConfig
	if err := config.LoadConfig(m.ConfigName, &commandConfig); err != nil {
		return nil
	}
	m.Config = &commandConfig

	if !commandConfig.Enabled {
		config.Logger.Infoln("Command feature disabled in configs")
		return nil
	}

	//m.Session.AddHandler(m.handleInteractionCreate)

	config.Logger.Infoln(m.Name(), "initialized!")
	return nil
}
