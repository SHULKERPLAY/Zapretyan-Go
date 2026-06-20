package dissender

import (
	"context"
	"discord-sender/internal/cfg"
	"discord-sender/internal/geomanager"
	"discord-sender/internal/util"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// SendEvent sends message to discord channel with 500ms pause.
// Requires bot.client, ChannelID, Message Text, embeds (if needed), Supress message embeds such as links (if needed)
func SendEvent(client bot.Client, channelID snowflake.ID, content string, embeds []discord.Embed, hideEmbeds bool) bool {
	// Exit if no channelID
	if channelID == 0 {
		return false
	}

	// Cut content if more than 1900 characters
	if len(content) > 1900 {
		content = content[:1900] + "\n..."
	}

	// Initialize message flags
	var flags discord.MessageFlags
	if hideEmbeds {
		flags = discord.MessageFlagSuppressEmbeds
	}

	// Create message object
	messageCreate := discord.MessageCreate{
		Content: content,
		Embeds:  embeds,
		Flags:   flags,
	}

	// Send builded message
	_, err := client.Rest.CreateMessage(channelID, messageCreate)
	if err != nil {
		util.LogMsg("Error while sending message to %s: %v", channelID, err)
		return false
	}

	// Sleep 500ms after each message
	util.LogMsg("Message sent in '%v'", channelID)
	time.Sleep(500 * time.Millisecond)
	return true
}

// CreateEmbed works as embed constructor.
// Reqires: Title, Description, Footer, color (e.g. 0x00c8ff), Author Name, Link to author Icon, set timestamp to footer (if needed).
func CreateEmbed(title, data, footer string, color int, authorName, authorIcon, authorLink string, timestamp bool) discord.Embed {
	// Cut description if more than 3900 characters
	if len(data) > 3900 {
		data = data[:3900] + "\n..."
	}

	// Set footer
	if footer == "" {
		footer = fmt.Sprintf("🩵 @%s", cfg.Self.BotTag)
	}

	// Set color
	if color == 0 {
		color = 0x5e87ff
	}

	// Build Embed
	embed := discord.NewEmbed() // Builder
	embed.Title = title			// Title
	embed.Description = data	// Description
	embed.Color = color			// Color
	embed.Footer = &discord.EmbedFooter{Text: footer} // Footer
	if timestamp { 
		// Set current timestamp in footer
		now := time.Now()
		embed.Timestamp = &now
	}

	// Set author
	if authorName != "" {
		embed.Author = &discord.EmbedAuthor{Name: authorName, URL: authorLink, IconURL: authorIcon}
	}

	return embed
}

// ProcessDomains processing Banned and Unbanned Domains lists to send
func ProcessDomains(ctx context.Context, client bot.Client, channelID snowflake.ID, title string, domains []string, todayCount, totalCount, embedColor int) {
	// Category filters. Counts and replaces matches with one category
	var DomainFilters = map[string][]string{
		fmt.Sprintf("%s", cfg.Loc.Casinos): {"casino", "kazino", "melbet", "mostbet", "1xbet", "1xslots", "1win", "admiralx", "pokerdom", "pinco", "pinup", "fortuna", "onlybets", "riobet", "vavada", "vulkan"},
		fmt.Sprintf("%s", cfg.Loc.Films): {"kino", "film", "serial"},
		// Here can be more categories in future. They are dynamic and do not require additional code
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// Initialize category counters
	categoryCounts := make(map[string]int)
	var filteredDomains []string

	// 1. Filtering
	// For every domain
	for _, domain := range domains {
		// If matched it sets to true
		matched := false
		// Search for every category
		for category, keywords := range DomainFilters {
			// Search for every keyword in category
			for _, kw := range keywords {
				// If found
				if strings.Contains(strings.ToLower(domain), kw) {
					// Increment current category counter
					categoryCounts[category]++
					matched = true
					// Stop cycle if domain matched
					break
				}
			}
			// Stop cycle if domain matched to category
			if matched {
				break
			}
		}
		// If no matches domain goes to common list
		if !matched {
			filteredDomains = append(filteredDomains, domain)
		}
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// 2. Build first block (Categories)
	var categoryBlock strings.Builder
	
	// For every category
	for cat, count := range categoryCounts {
		categoryBlock.WriteString(fmt.Sprintf("**`%s`**: %d\n", cat, count))
	}
	// If no categories write newline
	if categoryBlock.Len() > 0 {
		categoryBlock.WriteString("\n")
	}

	// Building chunks (Split by parts max 3800 characters)
	var chunks []string
	var currentChunk strings.Builder
	// Write categories in first chunk
	currentChunk.WriteString(categoryBlock.String())

	// For every common domain
	for _, d := range filteredDomains {
		// +1 For newline character
		if currentChunk.Len()+len(d)+1 > 3800 {
			// If length exceeded put current chunk to chunks array
			chunks = append(chunks, currentChunk.String())
			// Reset chunk
			currentChunk.Reset()
		}
		// Write domain
		currentChunk.WriteString(d)
		// Write newline
		currentChunk.WriteByte('\n') 
	}
	// If no more domains put current chunk to chunks array
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	// 4. Send Embeds
	// For every chunk we collected
	for i, chunk := range chunks {
		// Check if context closed
		if ctx.Err() != nil {
			return
		}

		// Set title
		embedTitle := fmt.Sprintf("📙 %s (%s)", title, cfg.Loc.Domains)
		// Add to title current and last page
		if i > 0 {
			embedTitle = embedTitle + fmt.Sprintf(" (%d/%d)", i+1, len(chunks))
		}

		// Create current chunk embed
		embed := CreateEmbed(embedTitle, chunk, cfg.Loc.Footer, embedColor, cfg.Embed.AuthorName, cfg.Embed.Icon, cfg.Embed.AuthorURL, true)
		// Send it
		SendEvent(client, channelID, "", []discord.Embed{embed}, false)
	}

	// 5. Final message
	var summary string 
	if totalCount > 0 { // Total Banned count provided to Banned Lists ONLY
		summary = fmt.Sprintf("🔥 **%s: __%d__!**\n", cfg.Loc.Banned, todayCount)
		summary += fmt.Sprintf("🚫 %s (%s): __%d__\n", cfg.Loc.TotalBan, cfg.Loc.Domains, totalCount)
	} else {
		summary = fmt.Sprintf("🔷 **%s: __%d__!**\n", cfg.Loc.Unbanned, todayCount)
	}
	SendEvent(client, channelID, summary, nil, false)
}

// ProcessIPs processing Banned and Unbanned IP lists to send
func ProcessIPs(ctx context.Context, client bot.Client, channelID snowflake.ID, title string, ips []string, todayCount, totalCount, embedColor int) {
	// Check if context closed
	if ctx.Err() != nil {
		return
	}
	
	// Init ASN and country counters for new message format
	asnCounts := make(map[string]int)
	countryCounts := make(map[string]int)

	// 1. Collect data for every IP
	// For every IP
	for _, ip := range ips {
		// Get ASN, Country and Filtered ISP Name
		info := geomanager.GeoService.GetIPInfo(ip)
		// If found info
		if info != nil {
			// Increment counter for combination "ISPName (ASN)""
			asnCounts[fmt.Sprintf("%s (AS%d)", info.Provider, info.ASN)]++
			countryCounts[info.Country]++
		}
	}

	// 2. Struct to sort collected data
	type kv struct {
		Key   string // ISP+ASN / Country
		Value int	 // Number of matches
	}

	// Sort ASN (Top 20)
	var asnList []kv
	// For every ISP
	for k, v := range asnCounts {
		// Add to sorting array
		asnList = append(asnList, kv{k, v})
	}
	// Sort collected slice
	sort.Slice(asnList, func(i, j int) bool { return asnList[i].Value > asnList[j].Value })
	// If length more than 20 cut matches to top 20
	if len(asnList) > 20 {
		asnList = asnList[:20]
	}

	// Sort countries (Top 10)
	var countryList []kv
	// For every country
	for k, v := range countryCounts {
		// Add to sorting array
		countryList = append(countryList, kv{k, v})
	}
	// Sort collected slice
	sort.Slice(countryList, func(i, j int) bool { return countryList[i].Value > countryList[j].Value })
	// If length more than 10 cut matches to top 10
	if len(countryList) > 10 {
		countryList = countryList[:10]
	}

	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// 3. Format Embed's body
	var desc strings.Builder
	// Write description header
	desc.WriteString(fmt.Sprintf("**__%s (20)__:**\n", cfg.Loc.TopIsp))
	// Fore every item in top ISP+ASN
	for _, item := range asnList {
		desc.WriteString(fmt.Sprintf("`%s`: **%d IP**\n", item.Key, item.Value))
	}
	// Write description header
	desc.WriteString(fmt.Sprintf("\n**__%s (10)__:**\n", cfg.Loc.TopCntry))
	// Fore every item in top country
	for _, item := range countryList {
		desc.WriteString(fmt.Sprintf("%s: **%d IP**\n", item.Key, item.Value))
	}

	// 4. Create embed
	embed := CreateEmbed(fmt.Sprintf("📙 %s (%s)", title, cfg.Loc.Ips), desc.String(), cfg.Loc.Footer, embedColor, cfg.Embed.AuthorName, cfg.Embed.Icon, cfg.Embed.AuthorURL, true)
	// Send new message with embed
	SendEvent(client, channelID, "", []discord.Embed{embed}, false)

	// 5. Final message
	var summary string
	if totalCount > 0 { // Total Banned count provided to Banned Lists ONLY
		summary = fmt.Sprintf("🟠 **%s: __%d__!**\n", cfg.Loc.Banned, todayCount)
		summary += fmt.Sprintf("❌ %s (%s): __%d__\n", cfg.Loc.TotalBan, cfg.Loc.Ips, totalCount)
	} else {
		summary = fmt.Sprintf("🔵 **%s: __%d__!**\n", cfg.Loc.Unbanned, todayCount)
	}
	// Send message
	SendEvent(client, channelID, summary, nil, false)
}

// Struct to read "Daily Statistics" plugin JSON
type DailyStats struct {
	TodayBan      string `json:"todayban"`
	TodayUnban    string `json:"todayunban"`
	TotalBan      string `json:"totalban"`
	TodayIPBan    string `json:"todayipban"`
	TodayIPUnban  string `json:"todayipunban"`
	TotalIPBan    string `json:"totalipban"`
}

// SendDailyStats reads "Daily Statistics" plugin JSON and send total day statistics.
// Creates a marker file to know when last message was sent. Skip if we already sent actual JSON data.
func SendDailyStats(ctx context.Context, client bot.Client, channelID snowflake.ID, jsonPath, markerPath string, embedColor int) {
	// Check if context closed
	if ctx.Err() != nil {
		return
	}

	// Check if JSON exists
	jsonStat, err := os.Stat(jsonPath)
	if err != nil {
		util.LogMsg("JSON file not found", err)
		return
	}

	// Check if marker exists
	markerStat, err := os.Stat(markerPath)
	// If marker exist and JSON older than marker then doing nothing
	if err == nil && !jsonStat.ModTime().After(markerStat.ModTime()) {
		return 
	}

	// Read Provided JSON
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return
	}

	// Parse JSON data into variable
	var stats DailyStats
	if err := json.Unmarshal(data, &stats); err != nil {
		util.LogMsg("Error parsing JSON:", err)
		return
	}

	// Set Date format
	dateStr := time.Now().Format("02/01/06")
	
	// Set embed description format
	desc := fmt.Sprintf(`**__%s__**
		🔥 %s: __%s__!
		🔷 %s: __%s__!
		🚫 %s: __%s__!

		**__%s__**
		🟠 %s: __%s__!
		🟢 %s: __%s__!
		❌ %s: __%s__!`,
	strings.ToUpper(cfg.Loc.Domains), cfg.Loc.DayBan, stats.TodayBan, cfg.Loc.DayUnban, stats.TodayUnban, cfg.Loc.TotalBan, stats.TotalBan, 
	strings.ToUpper(cfg.Loc.Ips), cfg.Loc.DayBan, stats.TodayIPBan, cfg.Loc.DayUnban, stats.TodayIPUnban, cfg.Loc.TotalBan, stats.TotalIPBan)

	// Create new embed
	embed := CreateEmbed(fmt.Sprintf("📌 %s %s", cfg.Loc.StatsDate, dateStr), desc, cfg.Loc.Footer, embedColor, cfg.Embed.AuthorName, cfg.Embed.Icon, cfg.Embed.AuthorURL, true)
	
	// Send message with embed
	success := SendEvent(client, channelID, "", []discord.Embed{embed}, false)

	// If message sent - update marker to not duplicate info until JSON updates
	if success {
		// Create or override dummy file with current time
		os.WriteFile(markerPath, []byte(""), 0644)
	}
}