package cfg

import (
	"context"
	"discord-sender/internal/util"
	"os"
	"path/filepath"
	"time"
)

type PluginBuild struct {
	Ver 	string // Plugin version
	JsonVer int	   // Supported Version of JSON proto of Zapretyan-Go
	Mode 	string // Plugin Mode
}

// PluginConfig - Part of structure that stores in "cfg"
type PluginConfig struct {
	NoMMDB   bool							   // True when errors trying to open readers of GeoLiteDB
	Ready    bool							   // True when Discord Bot is logged in
	ReadyCfg bool							   // True when config is loaded
	Path     string							   // Core Data directory
	BotTag	 string							   // Bot User Tag after Login
	BotToken string         `json:"BOT_TOKEN"` // Discord Bot Token
	Sender   *SenderConfig   `json:"sender"`   // [extension.sender] Enable message types
	Data     *DataConfig     `json:"data"`	   // [extension.data] Data sources for send
	Channels *ChannelsConfig `json:"channels"` // [extension.channels] Chat id's to send lists
	Embed    *EmbedConfig    `json:"embed"`    // [extension.embed] Override internal embed states
	Locale   *LocaleObject   `json:"locale"`   // [extension.locale] Override output messages text
}

type SenderConfig struct {
	Ban     bool `json:"isban"`		// Whether to send Banned Domains
	Unban   bool `json:"isunban"`	// Whether to send Unbanned Domains
	BanIp   bool `json:"isbanip"`	// Whether to send Banned IPs
	UnbanIp bool `json:"isunbanip"` // Whether to send Unbanned IPs
	Total   bool `json:"istotal"`	// Whether to send Daily statistics (Sets to false if DataConfig.TotalJSON not found)
}

type DataConfig struct {
	MmdbUpdate bool   `json:"mmdb_update"`      // Whether to autoupdate GeoLiteDB from db-ip.com
	MmdbLang   string `json:"mmdb_lang"`		// Language of database requests (DB should support it)
	AsnDB      string `json:"mmdbasn_path"`		// Path to IP-ASN DB
	CountryDB  string `json:"mmdbcountry_path"`	// Path to IP-Country DB
	TotalJSON  string `json:"total_json_path"`	// Path to "Daily Statistics" plugin JSON
}

type ChannelsConfig struct {
	Ban     string `json:"bancid"`	   // Discord ChannelID to send Banned Domains
	Unban   string `json:"unbancid"`   // Discord ChannelID to send Unbanned Domains
	BanIp   string `json:"banipcid"`   // Discord ChannelID to send Banned IPs
	UnbanIp string `json:"unbanipcid"` // Discord ChannelID to send Unbanned IPs
	Total   string `json:"totalcid"`   // Discord ChannelID to send Daily statistics
}

type EmbedConfig struct {
	Icon       string `json:"iconurl"`			 // URL to author icon image
	AuthorName string `json:"embed_author_name"` // Author name
	AuthorURL  string `json:"embed_author_url"`  // Author URL
	Ban        string `json:"banclr"`			 // Embed color for Banned Domains
	Unban      string `json:"unbanclr"`			 // Embed color for Unbanned Domains
	BanIp      string `json:"banipclr"`			 // Embed color for Banned IPs
	UnbanIp    string `json:"unbanipclr"`		 // Embed color for Unbanned IPs
	Total      string `json:"totalclr"`			 // Embed color for Daily statistics
	BanClr     int 			 					 // Integer of color for Banned Domains
	UnbanClr   int 								 // Integer of color for Unbanned Domains
	BanIpClr   int 								 // Integer of color for Banned IPs
	UnbanIpClr int 		 						 // Integer of color for Unbanned IPs
	TotalClr   int 			 					 // Integer of color for Daily statistics
}

// Override Locales
type LocaleObject struct {			  		 // Default values
	Banned 	  string `json:"banned"`		 // Добавлено записей в реестр
	Unbanned  string `json:"unbanned"`       // далено записей из реестра
	Casinos   string `json:"cat_casino"` 	 // Зеркала казино и букмекеры
	DayBan    string `json:"today_banned"`   // Сегодня заблокировано
	DayUnban  string `json:"today_unbanned"` // Сегодня разблокировано
	Domains	  string `json:"domains"`		 // Домены
	Films 	  string `json:"cat_film"`       // Пиратские кино и сериалы
	Footer 	  string `json:"embed_footer"`   // (Embed Footer override)
	Ips		  string `json:"ips"`			 // IP Адреса
	StatsDate string `json:"stats_date"`	 // Статистика за
	TopCntry  string `json:"top_country"`	 // Топ стран
	TopIsp	  string `json:"top_isp"`		 // Топ провайдеров
	TotalBan  string `json:"total_banned"`   // Всего заблокировано
	RknAdded  string `json:"newbanned"`		 // В реестр Роскомнадзора добавлены
	RknRemoved string `json:"newunbanned"`	 // Из реестра Роскомнадзора удалены
}

// Declare pointers to config structure
var Build PluginBuild // nil here is critical so it is NOT pointer
var Self *PluginConfig
var Loc *LocaleObject
var Channel *ChannelsConfig
var Sender *SenderConfig 
var Embed *EmbedConfig
var Data *DataConfig

// Load and process all config data. Provide core Data Path
func LoadConfig(dataPath string) {
	// Check Self
	// Check botToken
	if Self.BotToken == "" {
		util.LogMsg("FATAL: BOT TOKEN IS MISSING")
		os.Exit(1)
	}
	
	// Check if we cannot parse path of data dir
	if dataPath == "" {
		util.LogMsg("Failed to read core data dir. Defaulting to './data'")
		Self.Path = "./data"
	} else {
		Self.Path = dataPath
	}

	// Skip Self.Sender

	// Check Self.Data
	Self.Data.MmdbLang = util.ValidateString(Self.Data.MmdbLang, "en")

	// Validate strings
	Self.Data.AsnDB = util.ValidateString(Self.Data.AsnDB, "./discord-sender/dbip-asn.mmdb")
	Self.Data.CountryDB = util.ValidateString(Self.Data.CountryDB, "./discord-sender/dbip-country.mmdb")
	Self.Data.TotalJSON = util.ValidateString(Self.Data.TotalJSON, "./statistics/latest.json")

	// Join to Core Path if relative
	Self.Data.AsnDB = util.SmartJoin(Self.Path, Self.Data.AsnDB)
	Self.Data.CountryDB = util.SmartJoin(Self.Path, Self.Data.CountryDB)
	Self.Data.TotalJSON = util.SmartJoin(Self.Path, Self.Data.TotalJSON)

	// Check if JSON exists
	if Self.Sender.Total {
		_, err := os.Stat(Self.Data.TotalJSON)
		if err != nil {
			util.LogMsg("Daily Statistics JSON file not found: %v", err)
			Self.Sender.Total = false
		}
	}

	// Automaticly create folders if they not exist
	if err := os.MkdirAll(filepath.Dir(Self.Data.AsnDB), 0755); err != nil {
		util.LogMsg("Error creating directory '%v': %v", Self.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(Self.Data.CountryDB), 0755); err != nil {
		util.LogMsg("Error creating directory '%v': %v", Self.Path, err)
	}
	if err := os.MkdirAll(filepath.Join(Self.Path, "discord-sender"), 0755); err != nil {
		util.LogMsg("Error creating directory '%v': %v", Self.Path, err)
	}

	// Check Self.Channels
	if Self.Sender.Ban {
		if Self.Channels.Ban == "" {
			util.LogMsg("Error: Ban channelID not specified!")
			Self.Sender.Ban = false
		}
	}
	if Self.Sender.Unban {
		if Self.Channels.Unban == "" {
			util.LogMsg("Error: Unban channelID not specified!")
			Self.Sender.Unban = false
		}
	}
	if Self.Sender.BanIp {
		if Self.Channels.BanIp == "" {
			util.LogMsg("Error: BanIp channelID not specified!")
			Self.Sender.BanIp = false
		}
	}
	if Self.Sender.UnbanIp {
		if Self.Channels.UnbanIp == "" {
			util.LogMsg("Error: UnbanIp channelID not specified!")
			Self.Sender.UnbanIp = false
		}
	}
	if Self.Sender.Total {
		if Self.Channels.Total == "" {
			util.LogMsg("Error: Total channelID not specified!")
			Self.Sender.Total = false
		}
	}

	// Check Self.Embed
	Self.Embed.AuthorName = util.ValidateString(Self.Embed.AuthorName, "Запретян-Go <3")
	if !util.IsValidURL(Self.Embed.Icon) {
		Self.Embed.Icon = "https://lunarcreators.ru/wp-content/uploads/2025/11/discordiconmini.webp"
	}
	if !util.IsValidURL(Self.Embed.AuthorURL) {
		Self.Embed.Icon = "https://discord.com/discovery/applications/907372459144147035"
	}
	// Validate and convert colors
	Self.Embed.BanClr = util.ParseHexColor(Self.Embed.Ban, 0xff5e5e)
	Self.Embed.UnbanClr = util.ParseHexColor(Self.Embed.Unban, 0x5e87ff)
	Self.Embed.BanIpClr = util.ParseHexColor(Self.Embed.BanIp, 0xffa45e)
	Self.Embed.UnbanIpClr = util.ParseHexColor(Self.Embed.UnbanIp, 0x5effac)
	Self.Embed.TotalClr = util.ParseHexColor(Self.Embed.Total, 0xffff7d)

	// Check Self.Locale
	Self.Locale.Banned = util.ValidateLength(Self.Locale.Banned, "Добавлено записей в реестр", 1, 32)
	Self.Locale.Casinos = util.ValidateLength(Self.Locale.Casinos, "Зеркала казино и букмекеры", 1, 32)
	Self.Locale.Films = util.ValidateLength(Self.Locale.Films, "Пиратские кино и сериалы", 1, 32)
	Self.Locale.Domains = util.ValidateLength(Self.Locale.Domains, "Домены", 1, 32)
	Self.Locale.Footer = util.ValidateLength(Self.Locale.Footer, "", 1, 32) // Empty. Default value with bot user tag replacing inside Embed Bulder func
	Self.Locale.Ips = util.ValidateLength(Self.Locale.Ips, "IP Адреса", 1, 32)
	Self.Locale.RknAdded = util.ValidateLength(Self.Locale.RknAdded, "В реестр Роскомнадзора добавлены", 1, 32)
	Self.Locale.RknRemoved = util.ValidateLength(Self.Locale.RknRemoved, "Из реестра Роскомнадзора удалены", 1, 32)
	Self.Locale.StatsDate = util.ValidateLength(Self.Locale.StatsDate, "Статистика за", 1, 32)
	Self.Locale.DayBan = util.ValidateLength(Self.Locale.DayBan, "Сегодня заблокировано", 1, 32)
	Self.Locale.DayUnban = util.ValidateLength(Self.Locale.DayUnban, "Сегодня разблокировано", 1, 32)
	Self.Locale.TopCntry = util.ValidateLength(Self.Locale.TopCntry, "Топ стран", 1, 32)
	Self.Locale.TopIsp = util.ValidateLength(Self.Locale.TopIsp, "Топ провайдеров", 1, 32)
	Self.Locale.TotalBan = util.ValidateLength(Self.Locale.TotalBan, "Всего заблокировано", 1, 32)
	Self.Locale.Unbanned = util.ValidateLength(Self.Locale.Unbanned, "Удалено записей из реестра", 1, 32)

	// Define pointers
	Loc = Self.Locale
	Channel = Self.Channels
	Sender = Self.Sender
	Embed = Self.Embed
	Data = Self.Data

	util.LogMsg("Config loaded")
}

// Pause goroutine on this function until config pointer stop being nil.
// On context close return false. False needed to return in form: 
// if ret := cfg.WaitConfig(ctx); ret != true {return}.
// Because if context is closed func ends and next line after call reads nil pointer
// then context cancel leading to nil pointer panic.
func WaitConfig(ctx context.Context) bool {
	for Self == nil {
		select {
		case <-ctx.Done():
			util.LogMsg("ERROR: Config is nil on context end!")
			util.StopStdinScanner() // Close stdin and cause plugin context cancel
			return false
		default:
			time.Sleep(500 * time.Millisecond) // Small pause
		}
	}
	return true
}