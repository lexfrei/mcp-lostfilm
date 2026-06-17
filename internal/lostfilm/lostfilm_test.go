package lostfilm

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func testScraper(t *testing.T) *Scraper {
	t.Helper()

	scraper, err := New(&Options{BaseURL: "https://www.lostfilm.tv/"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return scraper
}

func docFromString(t *testing.T, html string) *goquery.Document {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	return doc
}

func TestParseFeedTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		title       string
		wantShow    string
		wantOrig    string
		wantEpisode string
		wantSeason  int
		wantEpNum   int
	}{
		{
			name:        "full episode",
			title:       "Легенда о Vox Machina (The Legend of Vox Machina). Мы – его кровь. (S04E06)",
			wantShow:    "Легенда о Vox Machina",
			wantOrig:    "The Legend of Vox Machina",
			wantEpisode: "Мы – его кровь",
			wantSeason:  4,
			wantEpNum:   6,
		},
		{
			name:       "no season/episode",
			title:      "Какой-то фильм (Some Movie). Описание.",
			wantShow:   "Какой-то фильм",
			wantOrig:   "Some Movie",
			wantSeason: 0,
			wantEpNum:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			show, orig, episode, season, epNum := parseFeedTitle(tt.title)
			if show != tt.wantShow {
				t.Errorf("show = %q, want %q", show, tt.wantShow)
			}

			if orig != tt.wantOrig {
				t.Errorf("orig = %q, want %q", orig, tt.wantOrig)
			}

			if tt.wantEpisode != "" && episode != tt.wantEpisode {
				t.Errorf("episode = %q, want %q", episode, tt.wantEpisode)
			}

			if season != tt.wantSeason || epNum != tt.wantEpNum {
				t.Errorf("season/episode = %d/%d, want %d/%d", season, epNum, tt.wantSeason, tt.wantEpNum)
			}
		})
	}
}

func TestSplitPlayToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		token             string
		series, sea, epis int
		ok                bool
	}{
		{"197006006", 197, 6, 6, true},
		{"197006999", 197, 6, WholeSeasonEpisode, true},
		{"1234012003", 1234, 12, 3, true},
		{"12", 0, 0, 0, false},
	}

	for _, tt := range tests {
		series, sea, epis, ok := splitPlayToken(tt.token)
		if ok != tt.ok || series != tt.series || sea != tt.sea || epis != tt.epis {
			t.Errorf("splitPlayToken(%q) = %d,%d,%d,%t; want %d,%d,%d,%t",
				tt.token, series, sea, epis, ok, tt.series, tt.sea, tt.epis, tt.ok)
		}
	}
}

func TestParseSize(t *testing.T) {
	t.Parallel()

	gib, mib := float64(giB), float64(miB)

	tests := []struct {
		desc      string
		wantText  string
		wantBytes int64
	}{
		{"Видео: 1080p. Размер: 1.35 ГБ. Перевод: x", "1.35 ГБ", int64(1.35 * gib)},
		{"Видео: SD. Размер: 389.67 МБ. Перевод: x", "389.67 МБ", int64(389.67 * mib)},
		{"Размер: 2,5 ГБ", "2,5 ГБ", int64(2.5 * gib)},
		{"no size here", "", 0},
	}

	for _, tt := range tests {
		text, bytes := parseSize(tt.desc)
		if text != tt.wantText || bytes != tt.wantBytes {
			t.Errorf("parseSize(%q) = %q,%d; want %q,%d", tt.desc, text, bytes, tt.wantText, tt.wantBytes)
		}
	}
}

func TestQualityFromDesc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		label, desc, want string
	}{
		{"MP4", "anything", "MP4"},
		{"", "Видео: 1080p WEB-DLRip", quality1080},
		{"", "Видео: 720p WEB-DLRip", quality720},
		{"", "Видео: 2160p", quality4K},
		{"", "Видео: WEB-DLRip", qualitySD},
	}

	for _, tt := range tests {
		if got := qualityFromDesc(tt.label, tt.desc); got != tt.want {
			t.Errorf("qualityFromDesc(%q,%q) = %q, want %q", tt.label, tt.desc, got, tt.want)
		}
	}
}

func TestNormalizeSeriesLink(t *testing.T) {
	t.Parallel()

	const want = "/series/Peaky_Blinders"

	tests := []struct {
		in, want string
	}{
		{want, want},
		{"series/Peaky_Blinders", want},
		{want + "/", want},
		{want + "/seasons", want},
	}

	for _, tt := range tests {
		if got := normalizeSeriesLink(tt.in); got != tt.want {
			t.Errorf("normalizeSeriesLink(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

const torrentPageFixture = `<html><body>
<div class="inner-box--item">
  <div class="inner-box--label"></div>
  <div class="inner-box--link main"><a href="https://n.tracktor.site/td.php?s=SD">Скачать</a></div>
  <div class="inner-box--link sub"><a href="https://n.tracktor.site/td.php?s=SD">.torrent</a></div>
  <div class="inner-box--desc">Видео: WEB-DLRip. Размер: 389.67 МБ. Перевод: Многоголосый (LostFilm.TV)</div>
</div>
<div class="inner-box--item">
  <div class="inner-box--label"></div>
  <div class="inner-box--link main"><a href="https://n.tracktor.site/td.php?s=FHD">Скачать</a></div>
  <div class="inner-box--desc">Видео: 1080p WEB-DLRip. Размер: 1.35 ГБ. Перевод: Многоголосый (LostFilm.TV)</div>
</div>
</body></html>`

func TestParseTorrentVariants(t *testing.T) {
	t.Parallel()

	variants := parseTorrentVariants(docFromString(t, torrentPageFixture))
	if len(variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(variants))
	}

	if variants[0].Quality != qualitySD || variants[0].DownloadURL != "https://n.tracktor.site/td.php?s=SD" {
		t.Errorf("variant 0 = %+v", variants[0])
	}

	if variants[1].Quality != quality1080 || variants[1].SizeText != "1.35 ГБ" {
		t.Errorf("variant 1 = %+v", variants[1])
	}
}

const seasonsPageFixture = `<html><body>
<h1 class="title-ru" itemprop="name">Острые козырьки</h1>
<h2 class="title-en" itemprop="alternativeHeadline">Peaky Blinders</h2>
<img src="/Static/Images/197/Posters/t_shmoster_s6.jpg" class="cover" />
<div class="serie-block">
<table class="movie-parts-list"><tbody>
<tr>
  <td class="alpha"><div class="haveseen-btn" data-episode="197006005" data-season="197006999" data-code="197-6-5"></div></td>
  <td class="beta" onClick="goTo('/series/Peaky_Blinders/season_6/episode_5/',false)">6 сезон 5 серия</td>
  <td class="gamma"><div>Дорога в ад<br /><span class="gray-color2 small-text">The Road to Hell</span></div></td>
  <td class="zeta"><div class="external-btn" onClick="PlayEpisode('197006005')"></div></td>
</tr>
</tbody></table>
</div>
</body></html>`

func TestParseSeriesInfo(t *testing.T) {
	t.Parallel()

	scraper := testScraper(t)
	doc := docFromString(t, seasonsPageFixture)

	id, poster := scraper.parseSeriesPoster(doc)
	if id != 197 {
		t.Errorf("series id = %d, want 197", id)
	}

	if poster != "https://www.lostfilm.tv/Static/Images/197/Posters/image.jpg" {
		t.Errorf("poster = %q", poster)
	}

	episodes := scraper.parseEpisodes(doc)
	if len(episodes) != 1 {
		t.Fatalf("episodes = %d, want 1", len(episodes))
	}

	ep := episodes[0]
	if ep.SeriesID != 197 || ep.Season != 6 || ep.Episode != 5 {
		t.Errorf("episode ids = %d/%d/%d, want 197/6/5", ep.SeriesID, ep.Season, ep.Episode)
	}

	if ep.Title != "Дорога в ад" || ep.TitleOrig != "The Road to Hell" {
		t.Errorf("titles = %q / %q", ep.Title, ep.TitleOrig)
	}

	if ep.URL != "https://www.lostfilm.tv/series/Peaky_Blinders/season_6/episode_5/" {
		t.Errorf("url = %q", ep.URL)
	}
}

func TestParseCookieHeader(t *testing.T) {
	t.Parallel()

	cookies := parseCookieHeader("lf_session=abc; cf_clearance=xyz")
	if len(cookies) != 2 {
		t.Fatalf("cookies = %d, want 2", len(cookies))
	}

	if cookies[0].Name != "lf_session" || cookies[0].Value != "abc" {
		t.Errorf("cookie 0 = %s=%s", cookies[0].Name, cookies[0].Value)
	}
}

func TestParseBases_RejectsInsecure(t *testing.T) {
	t.Parallel()

	_, err := New(&Options{BaseURL: "http://www.lostfilm.tv/"})
	if err == nil {
		t.Error("expected an error for an http base URL")
	}
}
