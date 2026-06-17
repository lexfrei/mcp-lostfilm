package lostfilm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// seasonsSuffix is appended to a series link to reach its episode listing.
const seasonsSuffix = "/seasons"

// dataCodeParts is the number of "-"-separated fields in a haveseen-btn
// data-code attribute (seriesID-season-episode).
const dataCodeParts = 3

// goToRe captures the episode page path from a goTo('/series/...') onclick.
var goToRe = regexp.MustCompile(`goTo\('([^']+)'`)

// playEpisodeRe captures the packed id token from a PlayEpisode('...') onclick.
var playEpisodeRe = regexp.MustCompile(`PlayEpisode\('(\d+)'\)`)

// SeriesInfo returns a series' metadata and episode list from its seasons page.
func (s *Scraper) SeriesInfo(ctx context.Context, link string) (*SeriesInfo, error) {
	return runOnMirror(s, func() (*SeriesInfo, error) {
		return s.seriesInfo(ctx, link)
	})
}

func (s *Scraper) seriesInfo(ctx context.Context, link string) (*SeriesInfo, error) {
	link = normalizeSeriesLink(link)

	doc, err := s.getDoc(ctx, link+seasonsSuffix, nil)
	if err != nil {
		return nil, err
	}

	info := &SeriesInfo{
		Link:      link,
		Title:     strings.TrimSpace(doc.Find("h1.title-ru").First().Text()),
		TitleOrig: strings.TrimSpace(doc.Find("h2.title-en").First().Text()),
		Seasons:   doc.Find("div.serie-block").Length(),
		URL:       s.resolve(link, ""),
	}

	info.ID, info.PosterURL = s.parseSeriesPoster(doc)
	info.Episodes = s.parseEpisodes(doc)

	return info, nil
}

// parseSeriesPoster reads the series id from a cover image path and builds the
// canonical poster URL.
func (s *Scraper) parseSeriesPoster(doc *goquery.Document) (int, string) {
	src := doc.Find("img.cover").First().AttrOr("src", "")

	match := seriesIDRe.FindStringSubmatch(src)
	if match == nil {
		return 0, ""
	}

	seriesID, _ := strconv.Atoi(match[1])
	posterURL := s.resolve(fmt.Sprintf("/Static/Images/%d/Posters/image.jpg", seriesID), "")

	return seriesID, posterURL
}

// parseEpisodes extracts every episode row across all season blocks.
func (s *Scraper) parseEpisodes(doc *goquery.Document) []Episode {
	episodes := make([]Episode, 0)

	doc.Find("table.movie-parts-list tr").Each(func(_ int, row *goquery.Selection) {
		episode, ok := s.parseEpisodeRow(row)
		if ok {
			episodes = append(episodes, episode)
		}
	})

	return episodes
}

// parseEpisodeRow builds an Episode from a single listing row, returning
// ok=false for header rows or rows without an identifiable episode.
func (s *Scraper) parseEpisodeRow(row *goquery.Selection) (Episode, bool) {
	seriesID, season, episodeNum, ok := episodeIDs(row)
	if !ok {
		return Episode{}, false
	}

	titleRu, titleOrig := episodeTitles(row.Find("td.gamma").First())

	episode := Episode{
		SeriesID:  seriesID,
		Season:    season,
		Episode:   episodeNum,
		Title:     titleRu,
		TitleOrig: titleOrig,
	}

	if match := goToRe.FindStringSubmatch(row.Find("td.beta").AttrOr("onclick", "")); match != nil {
		episode.URL = s.resolve(match[1], "")
	}

	return episode, true
}

// episodeIDs reads the seriesID/season/episode for a row, preferring the
// haveseen-btn data-code ("197-6-5") and falling back to a PlayEpisode token.
func episodeIDs(row *goquery.Selection) (int, int, int, bool) {
	code := row.Find(".haveseen-btn").AttrOr("data-code", "")
	if parts := strings.Split(code, "-"); len(parts) == dataCodeParts {
		seriesID, err1 := strconv.Atoi(parts[0])
		season, err2 := strconv.Atoi(parts[1])
		episode, err3 := strconv.Atoi(parts[2])

		if err1 == nil && err2 == nil && err3 == nil {
			return seriesID, season, episode, true
		}
	}

	onclick := row.Find("div.external-btn").AttrOr("onclick", "")
	if match := playEpisodeRe.FindStringSubmatch(onclick); match != nil {
		return splitPlayToken(match[1])
	}

	return 0, 0, 0, false
}

// splitPlayToken splits a packed PlayEpisode token "SSSSSSSSSEEE" where the last
// three digits are the episode, the previous three the season, and the rest the
// series id (e.g. 197006006 -> 197, 6, 6).
func splitPlayToken(token string) (int, int, int, bool) {
	const tail = 3

	if len(token) <= 2*tail {
		return 0, 0, 0, false
	}

	seriesID, err1 := strconv.Atoi(token[:len(token)-2*tail])
	season, err2 := strconv.Atoi(token[len(token)-2*tail : len(token)-tail])
	episode, err3 := strconv.Atoi(token[len(token)-tail:])

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}

	return seriesID, season, episode, true
}

// episodeTitles splits a gamma cell ("Russian<br><span>Original</span>") into
// the localized and original-language episode titles.
func episodeTitles(gamma *goquery.Selection) (string, string) {
	titleOrig := strings.TrimSpace(gamma.Find("span").First().Text())

	full := strings.TrimSpace(gamma.Text())
	titleRu := strings.TrimSpace(strings.Replace(full, titleOrig, "", 1))

	return titleRu, titleOrig
}

// normalizeSeriesLink ensures a leading slash and strips a trailing slash or an
// existing /seasons suffix so the caller can append it cleanly.
func normalizeSeriesLink(link string) string {
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "/") {
		link = "/" + link
	}

	link = strings.TrimSuffix(link, "/")
	link = strings.TrimSuffix(link, seasonsSuffix)

	return strings.TrimSuffix(link, "/")
}
