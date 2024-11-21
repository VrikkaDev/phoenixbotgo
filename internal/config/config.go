package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/yosuke-furukawa/json5/encoding/json5"
	"go.uber.org/zap"
)

var Logger *zap.SugaredLogger

type configuration struct {
	BotStatus string
	BotPrefix string

	DiscordToken string

	GuildID string `json:"GuildID"`
}

var Configuration *configuration

var configpaths = []string{"./configs/"}

func Load() {
	slogger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	Logger = slogger.Sugar()

	defer slogger.Sync()

	err = godotenv.Load()
	if err != nil {
		Logger.Panic("Couldnt load .env")
	}

	Configuration = &configuration{
		BotStatus:    os.Getenv("bot_activity_type"),
		BotPrefix:    os.Getenv("bot_activity_text"),
		DiscordToken: os.Getenv("DISCORD_TOKEN"),
	}

	LoadConfig("config.json5", &Configuration)
}

func LoadConfig(filename string, config interface{}) error {

	var data []byte
	// Try config paths for file
	for _, fp := range configpaths {
		d, err := os.ReadFile(fp + filename)
		if err == nil {
			data = d
			break
		}
	}

	if data == nil {
		Logger.Error("couldnt find config file : ", filename, " in paths ", configpaths)
		return fmt.Errorf("couldnt find config file")
	}

	err := json5.Unmarshal(data, config)
	if err != nil {
		Logger.Error("couldnt read config file: ", filename, "  ", err)
	}
	return nil
}
