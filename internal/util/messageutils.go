package util

import (
	"phoenixbot/internal/config"

	"github.com/bwmarrin/discordgo"
)

type ClearMessagesOnChannelOptions struct {
	Blacklist []string // User IDs to exclude from deletion
	Whitelist []string // User IDs to include in deletion
	Before    string   // Message ID to fetch messages before (optional)
	After     string   // Message ID to fetch messages after (optional)
	Limit     int      // Maximum number of messages to delete (optional, default 100)
}

func ClearMessagesOnChannel(session *discordgo.Session, channelID string, options *ClearMessagesOnChannelOptions) error {
	if options == nil {
		options = &ClearMessagesOnChannelOptions{}
	}

	limit := options.Limit
	if limit == 0 {
		limit = 100
	}

	messages, err := session.ChannelMessages(channelID, limit, options.Before, options.After, "")
	if err != nil {
		return err
	}

	blacklistMap := make(map[string]struct{})
	for _, id := range options.Blacklist {
		blacklistMap[id] = struct{}{}
	}

	whitelistMap := make(map[string]struct{})
	for _, id := range options.Whitelist {
		whitelistMap[id] = struct{}{}
	}

	var messagesToDelete []string
	for _, msg := range messages {
		authorID := msg.Author.ID

		// Skip if blacklisted
		if _, blacklisted := blacklistMap[authorID]; blacklisted {
			continue
		}

		// Skip if the whitelist and the author isnt in it
		if len(whitelistMap) > 0 {
			if _, whitelisted := whitelistMap[authorID]; !whitelisted {
				continue
			}
		}

		messagesToDelete = append(messagesToDelete, msg.ID)
	}

	// Delete messages in chunks of 100 cuz discord limits
	for i := 0; i < len(messagesToDelete); i += 100 {
		end := i + 100
		if end > len(messagesToDelete) {
			end = len(messagesToDelete)
		}

		if err := session.ChannelMessagesBulkDelete(channelID, messagesToDelete[i:end]); err != nil {
			config.Logger.Infoln("Failed to delete messages in channel %s: %v", channelID, err)
		}
	}

	return nil
}
