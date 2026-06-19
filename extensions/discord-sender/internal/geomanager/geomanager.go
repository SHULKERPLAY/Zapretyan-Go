package geomanager

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"discord-sender/internal/cfg"
	"discord-sender/internal/downloader"

	"github.com/oschwald/geoip2-golang"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Paths to City and ASN GeoLite2 Databases
var CountryDB = path.Join(cfg.Self.Path, "dbip-country.mmdb")
var ASNDB = path.Join(cfg.Self.Path, "dbip-asn.mmdb")

// GeoService stores mmdb databases connections
type GeoServices struct {
	CountryDB *geoip2.Reader
	ASNDB  *geoip2.Reader
}

// Define Geoservice
var GeoService *GeoServices

// Initialize GeoManager on import
func StartGeoService(ctx context.Context) {
	now := time.Now()

	// Check and update
	UpdateGeoLite(ctx, fmt.Sprintf("https://download.db-ip.com/free/dbip-country-lite-%d-%02d.mmdb.gz", now.Year(), now.Month()), CountryDB)
	UpdateGeoLite(ctx, fmt.Sprintf("https://download.db-ip.com/free/dbip-asn-lite-%d-%02d.mmdb.gz", now.Year(), now.Month()), ASNDB)
	
	// Initialize service
	GeoService = NewGeoService("GeoLite2-City.mmdb", "GeoLite2-ASN.mmdb")
}

// NewGeoService opening MaxMind databases
func NewGeoService(cityPath, asnPath string) (*GeoServices) {
	defer slog.Debug("NewGeoService() ended")
	city, err := geoip2.Open(cityPath)
	if err != nil {
		slog.Error("Error while opening City Database!", "err", err)
		slog.Error("All features requiring GeoLite service disabled!")
		cfg.Self.NoMMDB = true
		return nil
	}

	asn, err := geoip2.Open(asnPath)
	if err != nil {
		city.Close()
		slog.Error("Error while opening ASN Database!", "err", err)
		slog.Error("All features requiring GeoLite service disabled!")
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
	defer slog.Debug("UpdateGeoLite() ended")
	slog.Debug("Target path", "path", targetPath)
	info, err := os.Stat(targetPath)

	// If file not found or it is older than 45 days
	if os.IsNotExist(err) || time.Since(info.ModTime()) > 45*24*time.Hour {
		slog.Warn("Database too old or not exist. Updating...")
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
		return fmt.Errorf("не удалось создать папки для пути: %w", err)
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
	defer slog.Debug("GetIPInfo() ended")

	// Validate IP
	ip := net.ParseIP(inputIP)
	if ip == nil {
		slog.Warn("Wrong IP format!", "ip", inputIP)
		return nil
	}

	result := &ResultData{IP: inputIP}

	slog.Info("Got mmdb request", "ip", inputIP)

	// Collecting data: City
	countryRecord, err := s.CountryDB.Country(ip)
	if err != nil {
		slog.Error("Error while reading MaxMind Country Database", "err", err)
		return nil
	}

	// Get country code from city
	isoCode := countryRecord.Country.IsoCode
	countryName := countryRecord.Country.Names[cfg.Data.MmdbLang] // Locale output
	if countryName == "" {
		countryName = "Unknown Country"
	}
	
	slog.Debug("Got country", "countryName", countryName)

	// Put Unicode country flag to its name
	if isoCode != "" {
		result.Country = fmt.Sprintf("%s %s", isoCodeToEmoji(isoCode), countryName)
	} else {
		result.Country = countryName
	}

	// Get ISP org and its ASN
	asnRecord, err := s.ASNDB.ASN(ip)
	if err != nil {
		// If IP not found in ASN base
		result.Provider = "Unknown Provider"
	} else {
		result.ASN = asnRecord.AutonomousSystemNumber
		rawOrg := asnRecord.AutonomousSystemOrganization
		
		// Convert raw ISP name
		result.Provider = processISPName(result.ASN, rawOrg)
	}

	return result
}

// GetIPASN returns only ASN uint from mmdb by query string 
func (s *GeoServices) GetIPASN (inputIP string) uint {
	defer slog.Debug("GetIPASN() ended")

	// Validate IP
	ip := net.ParseIP(inputIP)
	if ip == nil {
		slog.Warn("Wrong IP format!", "ip", inputIP)
		return 0
	}

	slog.Info("Got mmdb request", "ip", inputIP)
	var result uint

	// Get ASN
	asnRecord, err := s.ASNDB.ASN(ip)
	if err != nil {
		// If IP not found in ASN base
		result = 0
	} else {
		result = asnRecord.AutonomousSystemNumber
		slog.Debug("Got ASN", "ASN", result)
	}

	return result
}

// Returns raw ASN org name by provided IP
func (s *GeoServices) GetKnownASNOrg(rawip string) string {
	// Validate IP
	ip := net.ParseIP(rawip)
	if ip == nil {
		slog.Warn("Wrong IP format!", "ip", rawip)
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
func processISPName(asn uint, rawName string) string {
	defer slog.Debug("processISPName() ended")

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

	slog.Debug("Converted", "asn", asn, "raw", rawName, "to", clean)

	return clean
}

// isoCodeToEmoji translates country characters to unicode flags. e.g. "RU" to "🇷🇺"
func isoCodeToEmoji(countryCode string) string {
	defer slog.Debug("isoCodeToEmoji() ended")
	if len(countryCode) != 2 {
		return "🏳️"
	}
	countryCode = strings.ToUpper(countryCode)
	// Unicode magic: shift characters code to country flags range
	return string(rune(countryCode[0])+127397) + string(rune(countryCode[1])+127397)
}



