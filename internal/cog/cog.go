package cog

import "github.com/bwmarrin/discordgo"

type Cog interface {
	Name() string
	Init(s *discordgo.Session) error
}
