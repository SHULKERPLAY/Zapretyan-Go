package disclient

import (
	"context"
	"discord-sender/internal/cfg"
	"discord-sender/internal/util"
	"os"
	"sync"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

var BotClient *bot.Client

func Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done() // Send to main goroutine sign that client has stopped on return

	// Hold function until config is ready
	if ret := cfg.WaitConfig(ctx); ret != true { // nil pointer guard
		return
	}
	hold := util.HoldAction(ctx, &cfg.Self.ReadyCfg, 12, 5)
	if !hold {
		util.LogMsg("ERROR: CONFIGURATION WAITTIME EXCEEDED!")
		os.Stdin.Close() // Close stdin and cause plugin context cancel
		return
	}

	// Check if token Exist
	if cfg.Self.BotToken == "" {
		util.LogMsg("ERROR: DISCORD_TOKEN environment variable is not set")
		return
	}

	// Create disgo client
	// disgo.New() accepts token and config functions
	client, err := disgo.New(cfg.Self.BotToken,
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
	err = client.OpenGateway(ctx)
	if err != nil {
		util.LogMsg("error while connecting to gateway: %v", err)
		return
	}

	// Make client public
	BotClient = client

	// Await context end to close current Discord session
	<-ctx.Done()

	// Close connection before exit
	util.LogMsg("Closing Discord Connection...")
	client.Close(context.Background())
}
