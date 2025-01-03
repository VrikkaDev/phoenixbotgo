package cog

import (
	"fmt"
	"phoenixbot/internal/config"
	"phoenixbot/internal/discord"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type TicketGuildConfig struct {
	Messages struct {
		TicketCreated     string `json:"TicketCreated"`
		NoPermission      string `json:"NoPermission"`
		CloseTicketPrompt string `json:"CloseTicketPrompt"`
		AlreadyHasTicket  string `json:"AlreadyHasTicket"`

		TicketChannelMessage discord.MessageData `json:"TicketChannelMessage"`
		TicketCreateMessage  discord.MessageData `json:"TicketCreateMessage"`
	} `json:"Messages"`

	Channel  string            `json:"Channel"`
	AddRoles map[string]string `json:"AddRoles"`
	Enabled  bool              `json:"Enabled"`
}

type TicketConfig struct {
	Guilds map[string]*TicketGuildConfig `json:"Guilds"`
}

type TicketCog struct {
	Cog

	ConfigName string

	Session *discordgo.Session
	Config  *TicketConfig

	TicketUsers sync.Map // Maps user id to thread id
}

func (m *TicketCog) Name() string {
	return "TicketCog"
}

func (m *TicketCog) Init() error {

	var ticketConfig TicketConfig
	if err := config.LoadConfig(m.ConfigName, &ticketConfig); err != nil {
		return nil
	}
	m.Config = &ticketConfig

	for guild, tic := range m.Config.Guilds {
		if !config.IsGuildEnabled(guild) {
			continue
		}
		if !tic.Enabled {
			config.Logger.Infoln("Ticket feature disabled in config, on server ", guild)
			continue
		}

		discord.ClearMessagesOnChannel(m.Session, tic.Channel, nil)
		m.sendApplyMessage(guild, tic.Channel)
	}

	m.Session.AddHandler(m.handleInteractionCreate)

	config.Logger.Infoln(m.Name(), "initialized!")
	return nil
}

func (m *TicketCog) sendApplyMessage(guildID string, channelID string) {

	conf, ok := m.Config.Guilds[guildID]
	if !ok {
		return
	}

	message, err := discord.CreateMessageSend(conf.Messages.TicketCreateMessage)
	if err != nil {
		config.Logger.Errorln(err)
	}

	applyButton := discordgo.Button{
		Label:    "Apply",
		Style:    discordgo.PrimaryButton,
		CustomID: "create_ticket_button",
	}

	message.Components = []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{applyButton}}}

	_, err = m.Session.ChannelMessageSendComplex(channelID, message)
	if err != nil {
		config.Logger.Errorln("Error sending apply message:", err)
	}
}

func (m *TicketCog) handleInteractionCreate(session *discordgo.Session, interaction *discordgo.InteractionCreate) {

	if interaction.Type != discordgo.InteractionMessageComponent {
		return
	}

	customID := interaction.MessageComponentData().CustomID

	switch customID {
	case "create_ticket_button":
		m.handleCreateTicket(session, interaction.Interaction)
	case "close_ticket_button":
		m.handleCloseTicketPrompt(session, interaction.Interaction)
	case "confirm_close_ticket_button":
		m.handleConfirmCloseTicket(session, interaction.Interaction)
	case "cancel_close_ticket_button":
		m.handleCancelCloseTicket(session, interaction.Interaction)
	}
}

func (m *TicketCog) handleCreateTicket(session *discordgo.Session, interaction *discordgo.Interaction) {

	conf, ok := m.Config.Guilds[interaction.GuildID]
	if !ok {
		return
	}

	var userId string
	if interaction.User != nil {
		userId = interaction.User.ID
	} else if interaction.Member != nil && interaction.Member.User != nil {
		userId = interaction.Member.User.ID
	} else {
		config.Logger.Errorln("Failed to retrieve user information from interaction")
		session.InteractionRespond(interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Internal error: unable to identify user",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Check if user already has tikcet
	if _, alrd := m.TicketUsers.Load(userId); alrd {
		session.InteractionRespond(interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: conf.Messages.AlreadyHasTicket,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Create ticket thread
	channeldId := interaction.ChannelID
	threadName := fmt.Sprintf("ticket-%s", userId[:6])
	thread, err := session.ThreadStart(channeldId, threadName, discordgo.ChannelTypeGuildPrivateThread, 4320)
	if err != nil {
		config.Logger.Errorln("Failed to create thread: ", err)
		return
	}

	m.TicketUsers.Store(userId, thread.ID)
	responseMessage := strings.NewReplacer("{channel}", thread.Mention()).Replace(conf.Messages.TicketCreated)
	session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: responseMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	messend, err := discord.CreateMessageSend(conf.Messages.TicketChannelMessage)
	if err != nil {
		config.Logger.Errorln("error creating MessageSend of TickedChannelMessage: ", err)
	}

	replacer := strings.NewReplacer("{user_id}", userId)
	messend.Content = replacer.Replace(messend.Content)

	closeTicketButton := discordgo.Button{
		Label:    "Close ticket",
		Style:    discordgo.DangerButton,
		CustomID: "close_ticket_button",
	}

	messend.Components = []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{closeTicketButton}}}

	_, err = session.ChannelMessageSendComplex(thread.ID, messend)
	if err != nil {
		config.Logger.Errorln(err)
	}
}

func (m *TicketCog) handleCloseTicketPrompt(session *discordgo.Session, interaction *discordgo.Interaction) {

	conf, ok := m.Config.Guilds[interaction.GuildID]
	if !ok {
		return
	}

	closePrompt := conf.Messages.CloseTicketPrompt
	session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: closePrompt,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Confirm",
							Style:    discordgo.PrimaryButton,
							CustomID: "confirm_close_ticket_button",
						},
						discordgo.Button{
							Label:    "Cancel",
							Style:    discordgo.SecondaryButton,
							CustomID: "cancel_close_ticket_button",
						},
					},
				},
			},
		},
	})
}

func (m *TicketCog) handleConfirmCloseTicket(session *discordgo.Session, interaction *discordgo.Interaction) {
	userID := interaction.Member.User.ID
	threadID := interaction.ChannelID

	if _, err := session.ChannelDelete(threadID); err != nil {
		config.Logger.Errorln("Failed to delete thread: ", err)
		session.InteractionRespond(interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Failed to delete the ticket.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if _, exists := m.TicketUsers.Load(userID); exists {
		m.TicketUsers.Delete(userID)
	}
	session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Ticket successfully closed.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (m *TicketCog) handleCancelCloseTicket(session *discordgo.Session, interaction *discordgo.Interaction) {
	session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Ticket close canceled.",
			Components: []discordgo.MessageComponent{},
		},
	})
}
