package config

import (
	"os"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var Logger *zap.SugaredLogger

type configuration struct {
	BotStatus string
	BotPrefix string

	DiscordToken string
}

var Configuration *configuration

func Load() {
	slogger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	Logger = slogger.Sugar()

	err = godotenv.Load()
	if err != nil {
		Logger.Panic("Couldnt load file .env")
	}

	Configuration = &configuration{
		BotStatus:    os.Getenv("bot_activity_type"),
		BotPrefix:    os.Getenv("bot_activity_text"),
		DiscordToken: os.Getenv("DISCORD_TOKEN"),
	}
}
