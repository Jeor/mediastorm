package handlers

import (
	"encoding/json"
	"novastream/models"
	"regexp"
	"strconv"
	"strings"
)

var sportsResolutionLabel = regexp.MustCompile(`(?i)\b(4320[pi]|2160[pi]|1080[pi]|720[pi]|480[pi]|8k|4k|uhd|fhd)\b`)
var sportsBitrateLabel = regexp.MustCompile(`(?i)\b([0-9]+(?:\.[0-9]+)?)\s*(mbps|kbps)\b`)

// Only explicit quality labels are interpreted. No stream/network probing occurs.
// Conflicting labels use the lower advertised resolution rather than overclaiming.
func reportedSportsQuality(label string) *models.SportsReportedQuality {
	height := 0
	for _, token := range sportsResolutionLabel.FindAllString(strings.ToLower(label), -1) {
		value := 0
		switch token {
		case "8k":
			value = 4320
		case "4k", "uhd":
			value = 2160
		case "fhd":
			value = 1080
		default:
			value, _ = strconv.Atoi(strings.TrimRight(token, "pi"))
		}
		if height == 0 || value < height {
			height = value
		}
	}
	var bitrate int64
	for _, match := range sportsBitrateLabel.FindAllStringSubmatch(label, -1) {
		value, _ := strconv.ParseFloat(match[1], 64)
		factor := float64(1000)
		if strings.EqualFold(match[2], "mbps") {
			factor = 1000000
		}
		b := int64(value * factor)
		if b >= 10000 && b <= 1000000000 && (bitrate == 0 || b < bitrate) {
			bitrate = b
		}
	}
	if height == 0 && bitrate == 0 {
		return nil
	}
	return &models.SportsReportedQuality{ResolutionHeight: height, BitrateBps: bitrate, Origin: "label"}
}

// Known values precede unknown values, giving sorting a deterministic total order.
func compareSportsQuality(a, b *models.SportsReportedQuality) int {
	var ah, bh int
	var ab, bb int64
	if a != nil {
		ah = a.ResolutionHeight
		ab = a.BitrateBps
	}
	if b != nil {
		bh = b.ResolutionHeight
		bb = b.BitrateBps
	}
	if ah > bh {
		return -1
	}
	if ah < bh {
		return 1
	}
	if ab > bb {
		return -1
	}
	if ab < bb {
		return 1
	}
	return 0
}

var stremioDimensions = regexp.MustCompile(`(?i)^([0-9]{2,5})\s*x\s*([0-9]{2,5})$`)

// Metadata values may be strings, numbers, null, or malformed optional fields.
func stremioQualityText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

// Use explicit reported values, never provider scores or generic HD labels as
// a resolution. Conflicting claims retain the lower value, as labels do.
func reportedStremioQuality(stream stremioStream) *models.SportsReportedQuality {
	labels := []string{stream.Name, stream.Title, stream.Description}
	for _, raw := range []json.RawMessage{stream.Resolution, stream.Quality} {
		value := stremioQualityText(raw)
		if dimensions := stremioDimensions.FindStringSubmatch(value); dimensions != nil {
			value = dimensions[2]
		}
		if height, err := strconv.Atoi(value); err == nil {
			switch height {
			case 480, 720, 1080, 2160, 4320:
				value += "p"
			default:
				value = ""
			}
		}
		labels = append(labels, value)
	}
	bitrate := stremioQualityText(stream.Bitrate)
	if value, err := strconv.ParseFloat(bitrate, 64); err == nil {
		// Numeric bitrate metadata is bits per second; do not guess Mbps units.
		if value >= 10000 && value <= 1000000000 {
			bitrate = strconv.FormatFloat(value/1000, 'f', -1, 64) + " kbps"
		} else {
			bitrate = ""
		}
	}
	labels = append(labels, bitrate)
	return reportedSportsQuality(strings.Join(labels, " "))
}
