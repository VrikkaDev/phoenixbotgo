package cog

import (
	"fmt"
	"phoenixbot/internal/config"
	"phoenixbot/internal/util"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type TicketConfig struct {
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

	if !ticketConfig.Enabled {
		config.Logger.Infoln("Ticket feature disabled in configs")
		return nil
	}

	m.Session.AddHandler(m.handleInteractionCreate)

	chn := strconv.Itoa(m.Config.Channel)
	util.ClearMessagesOnChannel(m.Session, chn, nil)
	m.sendApplyMessage(chn)

	config.Logger.Infoln(m.Name(), "initialized!")
	return nil
}

func (m *TicketCog) sendApplyMessage(channelID string) {
	message, err := util.CreateMessageSend(m.Config.Messages.TicketCreateMessage)
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
				Content: m.Config.Messages.AlreadyHasTicket,
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
	responseMessage := strings.NewReplacer("{channel}", thread.Mention()).Replace(m.Config.Messages.TicketCreated)
	session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: responseMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	messend, err := util.CreateMessageSend(m.Config.Messages.TicketChannelMessage)
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
	closePrompt := m.Config.Messages.CloseTicketPrompt
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

	threadID, exists := m.TicketUsers.Load(userID)
	if !exists {
		session.InteractionRespond(interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active ticket found.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if _, err := session.ChannelDelete(threadID.(string)); err != nil {
		config.Logger.Errorln("Failed to delete thread: ", err)
		return
	}
	m.TicketUsers.Delete(userID)
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
