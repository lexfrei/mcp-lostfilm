package lostfilm

import (
	"context"
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// rssPath is the public release feed, relative to the active mirror.
const rssPath = "/rss.xml"

// Title/description parsing patterns for RSS entries.
var (
	// seasonEpisodeRe captures the season and episode from a "(S04E06)" suffix.
	seasonEpisodeRe = regexp.MustCompile(`\(S(\d+)E(\d+)\)`)
	// showOrigRe captures the localized show name and its original-language name
	// from a "Show (Original)." prefix.
	showOrigRe = regexp.MustCompile(`^(.+?)\s*\(([^)]+)\)\.\s*(.*)$`)
	// posterSrcRe captures the poster image src from an RSS description.
	posterSrcRe = regexp.MustCompile(`src="([^"]+)"`)
	// seriesIDRe captures the lostfilm series id from a poster image path.
	seriesIDRe = regexp.MustCompile(`/Images/(\d+)/`)
)

// rssFeed mirrors the subset of the RSS document the feed parser reads.
type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// Feed returns the latest releases from the public RSS feed.
func (s *Scraper) Feed(ctx context.Context) ([]FeedItem, error) {
	return runOnMirror(s, func() ([]FeedItem, error) {
		return s.feed(ctx)
	})
}

func (s *Scraper) feed(ctx context.Context) ([]FeedItem, error) {
	body, err := s.getString(ctx, rssPath, nil)
	if err != nil {
		return nil, err
	}

	var feed rssFeed

	xmlErr := xml.Unmarshal([]byte(body), &feed)
	if xmlErr != nil {
		return nil, errors.Wrap(ErrParse, "decode RSS feed")
	}

	items := make([]FeedItem, 0, len(feed.Items))
	for i := range feed.Items {
		items = append(items, s.parseFeedItem(&feed.Items[i]))
	}

	return items, nil
}

// parseFeedItem converts one raw RSS item into a FeedItem, extracting the
// show/episode names, season/episode numbers, and poster/series id.
func (s *Scraper) parseFeedItem(raw *rssItem) FeedItem {
	item := FeedItem{
		Title:       strings.TrimSpace(raw.Title),
		URL:         strings.TrimSpace(raw.Link),
		PublishedAt: parsePubDate(raw.PubDate),
	}

	item.Show, item.ShowOrig, item.EpisodeName, item.Season, item.Episode = parseFeedTitle(item.Title)
	item.SeriesID, item.PosterURL = s.parsePoster(raw.Description)

	return item
}

// parseFeedTitle splits a feed title of the form
// "Show (Original). Episode name. (S04E06)" into its parts. Missing parts are
// returned empty/zero, so movie or whole-season entries degrade gracefully.
func parseFeedTitle(title string) (string, string, string, int, int) {
	var season, episodeNum int

	working := title

	if match := seasonEpisodeRe.FindStringSubmatch(working); match != nil {
		season, _ = strconv.Atoi(match[1])
		episodeNum, _ = strconv.Atoi(match[2])
		working = strings.TrimSpace(working[:strings.LastIndex(working, match[0])])
	}

	parts := showOrigRe.FindStringSubmatch(working)
	if parts == nil {
		return strings.TrimSpace(working), "", "", season, episodeNum
	}

	show := strings.TrimSpace(parts[1])
	orig := strings.TrimSpace(parts[2])
	episode := strings.TrimRight(strings.TrimSpace(parts[3]), ".")

	return show, orig, episode, season, episodeNum
}

// parsePoster extracts the series id and absolute poster URL from an RSS
// description's <img> tag.
func (s *Scraper) parsePoster(description string) (int, string) {
	src := posterSrcRe.FindStringSubmatch(description)
	if src == nil {
		return 0, ""
	}

	posterURL := s.resolve(src[1], "")

	var seriesID int
	if match := seriesIDRe.FindStringSubmatch(src[1]); match != nil {
		seriesID, _ = strconv.Atoi(match[1])
	}

	return seriesID, posterURL
}

// parsePubDate parses an RSS RFC1123Z date, returning the zero time on failure.
func parsePubDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC1123Z, value)
	if err != nil {
		return time.Time{}
	}

	return parsed.UTC()
}
