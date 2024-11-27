package cog

import (
	"fmt"
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"

	"github.com/bwmarrin/discordgo"
)

type CommandData struct {
	Enabled         bool                `json:"Enabled"`
	Description     string              `json:"Description"`
	AllowedChannels map[string]string   `json:"Allowed_channels"` // Allowed channels (name and ID)
	Response        discord.MessageData `json:"Response"`
}

type CommandGuildConfig struct {
	Enabled  bool                   `json:"Enabled"`
	Commands map[string]CommandData `json:"Commands"`
}

type CommandConfig struct {
	Guilds map[string]*CommandGuildConfig `json:"Guilds"`
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

	for guild, com := range m.Config.Guilds {
		if !config.IsGuildEnabled(guild) {
			continue
		}
		if !com.Enabled {
			config.Logger.Infoln("Command feature disabled in config, on server ", guild)
			continue
		}

		m.Session.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
			config.Logger.Infoln("Registering commands for server", guild)
			if err := m.registerCommands(guild); err != nil {
				config.Logger.Errorf("Failed to register commands: %v", err)
			}
		})
	}

	m.Session.AddHandler(m.HandleInteraction)

	config.Logger.Infoln(m.Name(), "initialized!")
	return nil
}

func (m *CommandCog) registerCommands(guildID string) error {

	conf, ok := m.Config.Guilds[guildID]
	if !ok {
		return fmt.Errorf("couldnt find command config for guild %s", guildID)
	}

	if m.Config == nil || !conf.Enabled {
		return nil
	}

	for name, command := range conf.Commands {
		if !command.Enabled {
			continue
		}

		appCommand := &discordgo.ApplicationCommand{
			Name:        name,
			Description: command.Description,
		}

		_, err := m.Session.ApplicationCommandCreate(m.Session.State.User.ID, guildID, appCommand)
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

	conf, ok := m.Config.Guilds[interaction.GuildID]
	if !ok {
		config.Logger.Warnln("Couldnt find command config for guild ", interaction.GuildID)
		return
	}

	commandName := interaction.ApplicationCommandData().Name
	command, exists := conf.Commands[commandName]
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

	ms, err := discord.CreateMessageSend(command.Response)
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
