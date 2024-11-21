package cog

import (
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"
	"phoenixbot/internal/util"

	"github.com/bwmarrin/discordgo"
)

type CommandData struct {
	Enabled         bool              `json:"Enabled"`
	Description     string            `json:"Description"`
	AllowedChannels map[string]string `json:"Allowed_channels"` // Allowed channels (name and ID)
	Response        util.MessageData  `json:"Response"`
}

type CommandConfig struct {
	Enabled  bool                   `json:"Enabled"`
	Commands map[string]CommandData `json:"Commands"`
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

	m.Session.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
		config.Logger.Infoln("Bot is ready, registering commands...")
		if err := m.registerCommands(); err != nil {
			config.Logger.Errorf("Failed to register commands: %v", err)
		}
	})

	m.Session.AddHandler(m.HandleInteraction)

	config.Logger.Infoln(m.Name(), "initialized!")
	return nil
}

func (m *CommandCog) registerCommands() error {
	if m.Config == nil || !m.Config.Enabled {
		return nil
	}

	for name, command := range m.Config.Commands {
		if !command.Enabled {
			continue
		}

		appCommand := &discordgo.ApplicationCommand{
			Name:        name,
			Description: command.Description,
		}

		_, err := m.Session.ApplicationCommandCreate(m.Session.State.User.ID, config.Configuration.GuildID, appCommand)
		if err != nil {
			config.Logger.Errorf("Failed to register command '%s': %v", name, err)
			return err
		}

		config.Logger.Infoln("Succesfully registered command: ", name)
	}

	return nil
}

func (m *CommandCog) HandleInteraction(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction.Type != discordgo.InteractionApplicationCommand {
		return
	}

	commandName := interaction.ApplicationCommandData().Name
	command, exists := m.Config.Commands[commandName]
	if !exists || !command.Enabled {
		return
	}

	if !isChannelAllowed(interaction.ChannelID, command.AllowedChannels) {
		_ = session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "This command is not allowed in this channel.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	ms, err := util.CreateMessageSend(command.Response)
	if err != nil {
		config.Logger.Errorln(err)
		return
	}

	err = discord.SendInteractionResponse(session, interaction.Interaction, ms)
	if err != nil {
		config.Logger.Errorln(err)
	}
}

func isChannelAllowed(channelID string, allowedChannels map[string]string) bool {
	if len(allowedChannels) == 0 {
		return true
	}

	for _, allowedID := range allowedChannels {
		if allowedID == channelID {
			return true
		}
	}
	return false
}
