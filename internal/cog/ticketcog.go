package cog

import (
	"phoenixbot/internal/config"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type TicketCog struct {
	mutex   sync.Mutex
	session *discordgo.Session

	tickets map[string]string // Maps user id to thread id
}

func (m *TicketCog) Name() string {
	return "TicketCog"
}

func (m *TicketCog) Init(s *discordgo.Session) error {
	m.session = s

	//s.AddHandler()
	config.Logger.Infoln(m.Name(), "initialized!")

	return nil
}

func (m *TicketCog) loadMessageConfig() {

}
