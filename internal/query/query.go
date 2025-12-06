package query

import (
	"strings"
	"time"

	"github.com/lootek/yt-rpi-player/internal/config"
)

func Build(keywords []string, dateFormat, dateLocale string) string {
	if dateFormat == "" {
		dateFormat = config.DefaultDateFormat
	}

	datePart := time.Now().Format(dateFormat)
	if dateLocale != "" {
		datePart = localizeDate(datePart, dateLocale)
	}

	terms := append([]string{}, keywords...)
	terms = append(terms, datePart)
	return strings.Join(terms, " ")
}

func localizeDate(date, locale string) string {
	switch strings.ToLower(locale) {
	case "pl", "pl_pl", "pl-pl":
		// Genitive month names (used with day numbers) and abbreviations.
		return strings.NewReplacer(
			"January", "stycznia",
			"February", "lutego",
			"March", "marca",
			"April", "kwietnia",
			"May", "maja",
			"June", "czerwca",
			"July", "lipca",
			"August", "sierpnia",
			"September", "września",
			"October", "października",
			"November", "listopada",
			"December", "grudnia",
			"Jan", "sty",
			"Feb", "lut",
			"Mar", "mar",
			"Apr", "kwi",
			"Jun", "cze",
			"Jul", "lip",
			"Aug", "sie",
			"Sep", "wrz",
			"Oct", "paź",
			"Nov", "lis",
			"Dec", "gru",
		).Replace(date)
	default:
		return date
	}
}
