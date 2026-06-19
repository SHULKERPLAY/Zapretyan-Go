package cfg

// PluginConfig - Part of structure that stores in "cfg"
type PluginConfig struct {
	NoMMDB   bool
	Ready    bool
	Path     string
	BotTag	 string
	BotToken string         `json:"BOT_TOKEN"`
	Sender   SenderConfig   `json:"sender"`
	Data     DataConfig     `json:"data"`
	Channels ChannelsConfig `json:"channels"`
	Embed    EmbedConfig    `json:"embed"`
	Locale   LocaleObject   `json:"locale"`
}

type SenderConfig struct {
	Ban     bool `json:"isban"`
	Unban   bool `json:"isunban"`
	BanIp   bool `json:"isbanip"`
	UnbanIp bool `json:"isunbanip"`
	Total   bool `json:"istotal"`
}

type DataConfig struct {
	MmdbUpdate bool   `json:"mmdb_update"`
	MmdbLang   string `json:"mmdb_lang"`
	AsnDB      string `json:"mmdbasn_path"`
	CountryDB  string `json:"mmdbcountry_path"`
	TotalJSON  string `json:"total_json_path"`
}

type ChannelsConfig struct {
	Ban     string `json:"bancid"`
	Unban   string `json:"unbancid"`
	BanIp   string `json:"banipcid"`
	UnbanIp string `json:"unbanipcid"`
	Total   string `json:"totalcid"`
}

type EmbedConfig struct {
	Icon       string `json:"iconurl"`
	AuthorName string `json:"embed_author_name"`
	AuthorURL  string `json:"embed_author_url"`
	Ban        string `json:"banclr"`
	Unban      string `json:"unbanclr"`
	BanIp      string `json:"banipclr"`
	UnbanIp    string `json:"unbanipclr"`
	Total      string `json:"totalclr"`
}


type LocaleObject struct {
	StatsDate string `json:"stats_date"`
	///////
}

var Self *PluginConfig // Define config
var Loc *LocaleObject // Define pointer to locales
var Channel *ChannelsConfig
var Sender *SenderConfig 
var Embed *EmbedConfig
var Data *DataConfig
// Loc = &Self.Locale
// Channel = &Self.Channels
// Sender = &Self.Sender
// Embed = &Self.Embed
// Data = &Self.Data