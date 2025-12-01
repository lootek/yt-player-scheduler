package query

import (
	"strings"
	"time"

	"github.com/lootek/yt-rpi-player/internal/config"
)

func Build(keywords []string, dateFormat string) string {
	if dateFormat == "" {
		dateFormat = config.DefaultDateFormat
	}
	datePart := time.Now().Format(dateFormat)
	terms := append([]string{}, keywords...)
	terms = append(terms, datePart)
	return strings.Join(terms, " ")
}
