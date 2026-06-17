// Package lostfilm provides an HTTP client and scraper for lostfilm.tv.
//
// Discovery (the release feed and series search) is public and needs no
// credentials. Resolving and downloading the .torrent for an episode requires
// a session: the client logs in with an e-mail/password (or a pre-obtained
// lf_session cookie) and persists the session to disk for reuse. lostfilm
// serves UTF-8, so no charset transcoding is needed.
package lostfilm

import "time"

// SeriesType distinguishes a multi-episode series from a one-off movie. Both
// are returned by the same search endpoint and told apart by their link prefix.
const (
	// TypeSeries marks a /series/ entry.
	TypeSeries = "series"
	// TypeMovie marks a /movies/ entry.
	TypeMovie = "movie"
)

// WholeSeasonEpisode is the episode number lostfilm uses for a whole-season
// pack (the "999" in a PlayEpisode token such as 197006999).
const WholeSeasonEpisode = 999

// FeedItem is a single entry from the public /rss.xml release feed.
type FeedItem struct {
	// Title is the raw RSS title, e.g.
	// "Легенда о Vox Machina (The Legend of Vox Machina). Мы – его кровь. (S04E06)".
	Title string `json:"title"`
	// Show is the localized show name parsed from Title.
	Show string `json:"show,omitempty"`
	// ShowOrig is the original-language show name parsed from Title, when present.
	ShowOrig string `json:"showOrig,omitempty"`
	// EpisodeName is the episode title parsed from Title, when present.
	EpisodeName string `json:"episodeName,omitempty"`
	// SeriesID is the lostfilm series id parsed from the poster URL (0 if absent).
	SeriesID int `json:"seriesId,omitempty"`
	// Season and Episode are 0 when the entry is not a single episode (e.g. a
	// movie). Episode is WholeSeasonEpisode for a whole-season pack.
	Season  int `json:"season,omitempty"`
	Episode int `json:"episode,omitempty"`
	// URL is the absolute episode page URL.
	URL string `json:"url"`
	// PosterURL is the absolute poster URL (empty when no id could be parsed).
	PosterURL string `json:"posterUrl,omitempty"`
	// PublishedAt is the entry's pubDate; zero when it could not be parsed.
	PublishedAt time.Time `json:"publishedAt,omitzero"`
}

// Series is a single hit from the search endpoint.
type Series struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	TitleOrig string `json:"titleOrig,omitempty"`
	// Link is the site-relative path, e.g. "/series/Peaky_Blinders".
	Link string `json:"link"`
	// Type is TypeSeries or TypeMovie.
	Type string `json:"type"`
	// PosterURL is the absolute poster URL, when available.
	PosterURL string `json:"posterUrl,omitempty"`
	// URL is the absolute page URL.
	URL string `json:"url"`
}

// Episode is one row from a series' seasons page.
type Episode struct {
	SeriesID int `json:"seriesId"`
	Season   int `json:"season"`
	// Episode is WholeSeasonEpisode (999) for a whole-season pack.
	Episode   int    `json:"episode"`
	Title     string `json:"title,omitempty"`
	TitleOrig string `json:"titleOrig,omitempty"`
	// URL is the absolute episode page URL, when present.
	URL string `json:"url,omitempty"`
}

// SeriesInfo is the detailed view of a single series, including its episodes.
type SeriesInfo struct {
	ID          int       `json:"id,omitempty"`
	Title       string    `json:"title,omitempty"`
	TitleOrig   string    `json:"titleOrig,omitempty"`
	Link        string    `json:"link"`
	PosterURL   string    `json:"posterUrl,omitempty"`
	Description string    `json:"description,omitempty"`
	Seasons     int       `json:"seasons"`
	Episodes    []Episode `json:"episodes"`
	URL         string    `json:"url"`
}

// TorrentVariant is one quality option for an episode, resolved through the
// v_search redirect chain.
type TorrentVariant struct {
	// Quality is the short label, e.g. "1080p", "720p", "SD".
	Quality string `json:"quality,omitempty"`
	// Description is the raw "Видео: ... Размер: ... Перевод: ..." line.
	Description string `json:"description,omitempty"`
	// SizeText is the human-readable size parsed from Description (e.g. "1.35 ГБ").
	SizeText string `json:"sizeText,omitempty"`
	// SizeBytes is SizeText converted to bytes (0 when it could not be parsed).
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// DownloadURL is the n.tracktor.site/td.php link serving the .torrent.
	DownloadURL string `json:"downloadUrl"`
}

// TorrentFile is a downloaded .torrent payload.
type TorrentFile struct {
	Filename  string
	Content   []byte
	SizeBytes int
}
