package util

import (
	"phoenixbot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// Maybe should do something else instead of these util files
func GetUserVoiceState(s *discordgo.Session, guildID, userID string) *discordgo.VoiceState {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		config.Logger.Errorln("Failed to get guild:", err)
		return nil
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return vs
		}
	}
	return nil
}
