package geomanager

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"discord-sender/internal/cfg"
	"discord-sender/internal/downloader"
	"discord-sender/internal/util"

	"github.com/oschwald/geoip2-golang"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Paths to City and ASN GeoLite2 Databases
var CountryDB string
var ASNDB string

// GeoService stores mmdb databases connections
type GeoServices struct {
	CountryDB *geoip2.Reader
	ASNDB  *geoip2.Reader
}

// Define Geoservice
var GeoService *GeoServices

// Initialize GeoManager on import
func StartGeoService(ctx context.Context) {
	var CountryDB = cfg.Data.CountryDB
	var ASNDB = cfg.Data.AsnDB

	now := time.Now()

	// Check and update
	if cfg.Data.MmdbUpdate {
		UpdateGeoLite(ctx, fmt.Sprintf("https://download.db-ip.com/free/dbip-country-lite-%d-%02d.mmdb.gz", now.Year(), now.Month()), CountryDB)
		UpdateGeoLite(ctx, fmt.Sprintf("https://download.db-ip.com/free/dbip-asn-lite-%d-%02d.mmdb.gz", now.Year(), now.Month()), ASNDB)
	}
	
	// Initialize service
	GeoService = NewGeoService(CountryDB, ASNDB)
}

// NewGeoService opening MaxMind databases
func NewGeoService(countryPath, asnPath string) (*GeoServices) {
	city, err := geoip2.Open(countryPath)
	if err != nil {
		util.LogMsg("Error while opening Country Database: %v", err)
		util.LogMsg("All features requiring GeoLite service disabled!")
		cfg.Self.NoMMDB = true
		return nil
	}

	asn, err := geoip2.Open(asnPath)
	if err != nil {
		city.Close()
		util.LogMsg("Error while opening ASN Database!: %v", err)
		util.LogMsg("All features requiring GeoLite service disabled!")
		cfg.Self.NoMMDB = true
		return nil
	}

	return &GeoServices{CountryDB: city, ASNDB: asn}
}

// Closes MMDB connections
func (s *GeoServices) Close() {
	s.CountryDB.Close()
	s.ASNDB.Close()
}

// Checks for updates and download requested edition of GeoLiteDB
func UpdateGeoLite(ctx context.Context, link string, targetPath string) {
	info, err := os.Stat(targetPath)

	// If file not found or it is older than 45 days
	if os.IsNotExist(err) || time.Since(info.ModTime()) > 45*24*time.Hour {
		util.LogMsg("Database too old or not exist. Updating...")
		downloadAndExtractGeoLite(ctx, link, targetPath)
	}
}

// Download Gzip and unpack it as targetPath
func downloadAndExtractGeoLite(ctx context.Context, link, targetPath string) {
	gzip := fmt.Sprintf("%s.gz", targetPath)
	if err := downloader.DownloadFile(ctx, link, gzip); err != nil {
		return
	}
	if err := unpackGz(gzip, targetPath); err != nil {
		return
	}
	util.LogMsg("Database '%s' Updated", targetPath)
}

// UnpackGz accepts path to .gz archive and target path for .mmdb file.
// Unpacks and removing base GZIP archive.
func unpackGz(gzFilePath, outputMmdbPath string) error {
	// Remove archive on exit
	defer os.Remove(gzFilePath)

	// Open .gz archive
	gzFile, err := os.Open(gzFilePath)
	if err != nil {
		return fmt.Errorf("Error opening GZIP: %w", err)
	}
	defer gzFile.Close()

	// Create Gzip Reader
	gzipReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return fmt.Errorf("Error creating GZIP reader: %w", err)
	}
	defer gzipReader.Close()

	// Check if target path exist
	outputDir := filepath.Dir(outputMmdbPath)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("Error creating dir: %w", err)
	}

	// Create target .mmdb file
	outputFile, err := os.Create(outputMmdbPath)
	if err != nil {
		return fmt.Errorf("Error creating .mmdb file: %w", err)
	}
	defer outputFile.Close()

	// Unpack data
	_, err = io.Copy(outputFile, gzipReader)
	if err != nil {
		return fmt.Errorf("Error copying data: %w", err)
	}

	return nil
}

// ResultData is an reply structure
type ResultData struct {
	IP       string // Query IP Address
	Country  string // Country String e.g. "🇷🇺 Россия"
	Provider string // Filtered ISP name
	ASN      uint // ISP ASN. e.g. for Google Cloud it will be "15169"
}

// GetIPInfo collecting data from mmdb by query string
func (s *GeoServices) GetIPInfo(inputIP string) *ResultData {
	// Validate IP
	ip := net.ParseIP(inputIP)
	if ip == nil {
		util.LogMsg("Wrong IP format (%s)", inputIP)
		return nil
	}

	result := &ResultData{IP: inputIP}

	// Collecting data: Country
	countryRecord, err := s.CountryDB.Country(ip)
	if err != nil {
		util.LogMsg("Error while reading Country Database: %v", err)
		return nil
	}

	// Get country code
	isoCode := countryRecord.Country.IsoCode
	countryName := countryRecord.Country.Names[cfg.Data.MmdbLang] // Locale output
	if countryName == "" {
		countryName = "Unknown Country"
	}

	// Put Unicode country flag to its name
	if isoCode != "" {
		result.Country = fmt.Sprintf("%s %s", isoCodeToEmoji(isoCode), countryName)
	} else {
		result.Country = countryName
	}

	// Get ISP org and its ASN
	asnRecord, err := s.ASNDB.ASN(ip)
	if err != nil || asnRecord.AutonomousSystemNumber == 0 {
		// If IP not found in ASN base
		result.ASN = 0
		result.Provider = "Unknown ISP"
	} else { 
		result.ASN = asnRecord.AutonomousSystemNumber
		rawOrg := asnRecord.AutonomousSystemOrganization
		
		// Convert raw ISP name
		result.Provider = processISPName(rawOrg)
	}

	return result
}

// GetIPASN returns only ASN uint from mmdb by query string 
func (s *GeoServices) GetIPASN (inputIP string) uint {
	// Validate IP
	ip := net.ParseIP(inputIP)
	if ip == nil {
		return 0
	}

	var result uint

	// Get ASN
	asnRecord, err := s.ASNDB.ASN(ip)
	if err != nil {
		// If IP not found in ASN base
		result = 0
	} else {
		result = asnRecord.AutonomousSystemNumber
	}

	return result
}

// Returns raw ASN org name by provided IP
func (s *GeoServices) GetKnownASNOrg(rawip string) string {
	// Validate IP
	ip := net.ParseIP(rawip)
	if ip == nil {
		return ""
	}

	// Get ASN of IP
	asnRecord, err := s.ASNDB.ASN(ip)
	if err != nil {
		// If IP not found in ASN base
		return ""
	}

	// Return raw name if not match
	return asnRecord.AutonomousSystemOrganization
}

// processISPName cleans provider raw AS name to be more presentable
func processISPName(rawName string) string {
	// Clean raw name if ISP not in map
	// All to uppercase to suffix deletion
	clean := strings.ToUpper(rawName)
	
	// Erasing theese parts of Raw names
	replacer := strings.NewReplacer(
		" LLC", "",
		" INC.", "",
		" INC", "",
		" CORP.", "",
		" CORPORATION", "",
		" CORP", "",
		" LTD.", "",
		" LTD", "",
		" LIMITED", "",
		" GMBH", "",
		" S.R.L.", "",
		" BV", "",
		" N.V.", "",
		",", "", // Erase commas
		".", "", // Erase dots
		" INTERNATIONAL", "",
		" NETWORK", "",
		" NETWORKS", "",
		" SOLUTIONS", "",
		" TELECOM", "",
		" COMMUNICATIONS", "",
	)
	clean = replacer.Replace(clean)
	
	// Trim spaces
	clean = strings.TrimSpace(clean)

	// Convert to Title Case
	// strings.Title is deprecated but it will work for now
	clean = cases.Title(language.Und).String(strings.ToLower(clean))

	return clean
}

// isoCodeToEmoji translates country characters to unicode flags. e.g. "RU" to "🇷🇺"
func isoCodeToEmoji(countryCode string) string {
	if len(countryCode) != 2 {
		return "🏳️"
	}
	countryCode = strings.ToUpper(countryCode)
	// Unicode magic: shift characters code to country flags range
	return string(rune(countryCode[0])+127397) + string(rune(countryCode[1])+127397)
}



