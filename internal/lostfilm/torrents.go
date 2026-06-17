package lostfilm

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/cockroachdb/errors"
)

// vSearchPath is the authenticated endpoint that resolves an episode to its
// per-quality torrent links via an intermediate meta-refresh page.
const vSearchPath = "/v_search.php"

// loginPromptMarker is the text v_search.php returns when the session is absent.
const loginPromptMarker = "log in first"

// Binary size unit multipliers for parsing the "Размер" field.
const (
	kiB = 1 << 10
	miB = 1 << 20
	giB = 1 << 30
	tiB = 1 << 40
)

// Quality labels inferred from a torrent's video description.
const (
	quality4K   = "4K"
	quality1080 = "1080p"
	quality720  = "720p"
	qualitySD   = "SD"
)

var (
	// metaRefreshRe captures the redirect target from the v_search meta-refresh.
	metaRefreshRe = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']?refresh["']?[^>]+url=([^"'>]+)`)
	// sizeRe captures the size value and unit from a "Размер: 1.35 ГБ" fragment.
	sizeRe = regexp.MustCompile(`Размер:\s*([\d.,]+)\s*([\p{L}]+)`)
)

// Torrents resolves the available quality variants for an episode. Pass
// WholeSeasonEpisode (999) as episode for a whole-season pack.
func (s *Scraper) Torrents(ctx context.Context, seriesID, season, episode int) ([]TorrentVariant, error) {
	return runAuthed(ctx, s, func() ([]TorrentVariant, error) {
		return s.torrents(ctx, seriesID, season, episode)
	})
}

func (s *Scraper) torrents(ctx context.Context, seriesID, season, episode int) ([]TorrentVariant, error) {
	query := url.Values{
		"c": {strconv.Itoa(seriesID)},
		"s": {strconv.Itoa(season)},
		"e": {strconv.Itoa(episode)},
	}

	body, err := s.getString(ctx, vSearchPath, query)
	if err != nil {
		return nil, err
	}

	if strings.Contains(strings.ToLower(body), loginPromptMarker) {
		return nil, ErrNotAuthenticated
	}

	target := metaRefreshRe.FindStringSubmatch(body)
	if target == nil {
		return nil, errors.Wrap(ErrParse, "no meta-refresh in v_search response")
	}

	doc, err := s.followRedirect(ctx, target[1])
	if err != nil {
		return nil, err
	}

	variants := parseTorrentVariants(doc)
	if len(variants) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "no torrents for c=%d s=%d e=%d", seriesID, season, episode)
	}

	return variants, nil
}

// followRedirect fetches the meta-refresh target page (the /V/ download page)
// under the active mirror, preserving the signed query parameters.
func (s *Scraper) followRedirect(ctx context.Context, target string) (*goquery.Document, error) {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return nil, errors.Wrap(ErrParse, "parse meta-refresh URL")
	}

	return s.getDoc(ctx, parsed.Path, parsed.Query())
}

// parseTorrentVariants extracts the per-quality download options from a /V/
// download page's inner-box items.
func parseTorrentVariants(doc *goquery.Document) []TorrentVariant {
	variants := make([]TorrentVariant, 0)

	doc.Find("div.inner-box--item").Each(func(_ int, item *goquery.Selection) {
		href := item.Find("div.inner-box--link.main a").First().AttrOr("href", "")
		if href == "" {
			href = item.Find("div.inner-box--link a").First().AttrOr("href", "")
		}

		if href == "" {
			return
		}

		desc := strings.TrimSpace(item.Find("div.inner-box--desc").First().Text())
		label := strings.TrimSpace(item.Find("div.inner-box--label").First().Text())

		variant := TorrentVariant{
			Quality:     qualityFromDesc(label, desc),
			Description: desc,
			DownloadURL: href,
		}
		variant.SizeText, variant.SizeBytes = parseSize(desc)

		variants = append(variants, variant)
	})

	return variants
}

// qualityFromDesc returns the explicit quality label when present, otherwise
// infers it from the video description.
func qualityFromDesc(label, desc string) string {
	if label != "" {
		return label
	}

	switch {
	case strings.Contains(desc, "2160") || strings.Contains(desc, quality4K):
		return quality4K
	case strings.Contains(desc, "1080"):
		return quality1080
	case strings.Contains(desc, "720"):
		return quality720
	default:
		return qualitySD
	}
}

// parseSize extracts the human-readable size and its byte count from a
// description's "Размер" field.
func parseSize(desc string) (string, int64) {
	match := sizeRe.FindStringSubmatch(desc)
	if match == nil {
		return "", 0
	}

	text := match[1] + " " + match[2]

	num, err := strconv.ParseFloat(strings.Replace(match[1], ",", ".", 1), 64)
	if err != nil {
		return text, 0
	}

	return text, int64(num * float64(unitMultiplier(match[2])))
}

// unitMultiplier maps a size unit (Cyrillic or Latin) to its byte multiplier.
func unitMultiplier(unit string) int64 {
	switch strings.ToUpper(unit) {
	case "КБ", "KB":
		return kiB
	case "МБ", "MB":
		return miB
	case "ГБ", "GB":
		return giB
	case "ТБ", "TB":
		return tiB
	default:
		return 1
	}
}
