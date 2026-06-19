package disclient

import (
	"context"
	"discord-sender/internal/cfg"
	"discord-sender/internal/util"
	"sync"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

func Start(ctx context.Context, wg *sync.WaitGroup) {
	// Check if token Exist
	if cfg.Self.BotToken == "" {
		util.LogMsg("ERROR: DISCORD_TOKEN environment variable is not set")
		return
	}

	// Create disgo client
	// disgo.New() accepts token and config functions
	Client, err := disgo.New(cfg.Self.BotToken,
		// Setup discord gateway logic
		bot.WithGatewayConfigOpts(
			// Set intents for our bot.
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				// If we need to read messages add gateway.IntentMessageContent
			),
		),
		// Register Ready event processor
		bot.WithEventListenerFunc(func(e *events.Ready) {
			// Output bot tag after successful login
			botTag := e.EventReady.User.Tag()
			util.LogMsg("Logged in as %s", botTag)
			cfg.Self.BotTag = botTag
			cfg.Self.Ready = true
		}),
	)
	if err != nil {
		util.LogMsg("Error while building disgo client: %v", err)
		return
	}

	// Open Discord connection
	err = Client.OpenGateway(ctx)
	if err != nil {
		util.LogMsg("error while connecting to gateway: %v", err)
		return
	}

	// Await context end to close current Discord session
    <-ctx.Done()

	// Close connection before exit
	util.LogMsg("Closing Discord Connection...")
	Client.Close(context.Background())
}