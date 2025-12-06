package query

import (
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/lootek/yt-rpi-player/internal/config"
)

func Build(keywords []string, dateFormat string) string {
	if dateFormat == "" {
		dateFormat = config.DefaultDateFormat
	}

	locale := language.MustParse("pl")
	datePart := message.NewPrinter(locale).Sprintf(dateFormat, time.Now())

	terms := append([]string{}, keywords...)
	terms = append(terms, datePart)
	return strings.Join(terms, " ")
}
