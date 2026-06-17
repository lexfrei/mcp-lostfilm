package lostfilm

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// searchResponse mirrors the ajaxik.php search JSON. Both series and movies are
// returned under "series"; they are told apart by their link prefix.
type searchResponse struct {
	Data struct {
		Series []searchSeries `json:"series"`
	} `json:"data"`
	Result string `json:"result"`
}

type searchSeries struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	//nolint:tagliatelle // field name is dictated by the external lostfilm API.
	TitleOrig string `json:"title_orig"`
	Link      string `json:"link"`
	Icon      string `json:"icon"`
}

// Search returns series and movies matching query.
func (s *Scraper) Search(ctx context.Context, query string) ([]Series, error) {
	return runOnMirror(s, func() ([]Series, error) {
		return s.search(ctx, query)
	})
}

func (s *Scraper) search(ctx context.Context, query string) ([]Series, error) {
	body, err := s.postForm(ctx, ajaxikPath, url.Values{
		fieldAct:  {"common"},
		fieldType: {"search"},
		"val":     {query},
	})
	if err != nil {
		return nil, err
	}

	var resp searchResponse

	jsonErr := json.Unmarshal([]byte(body), &resp)
	if jsonErr != nil {
		return nil, errors.Wrap(ErrParse, "decode search response")
	}

	results := make([]Series, 0, len(resp.Data.Series))
	for i := range resp.Data.Series {
		results = append(results, s.toSeries(&resp.Data.Series[i]))
	}

	return results, nil
}

// toSeries maps a raw search hit to a Series, resolving its URLs and deriving
// the series/movie type from the link prefix.
func (s *Scraper) toSeries(raw *searchSeries) Series {
	seriesID, _ := strconv.Atoi(raw.ID)

	seriesType := TypeSeries
	if strings.HasPrefix(raw.Link, "/movies/") {
		seriesType = TypeMovie
	}

	series := Series{
		ID:        seriesID,
		Title:     raw.Title,
		TitleOrig: raw.TitleOrig,
		Link:      raw.Link,
		Type:      seriesType,
		URL:       s.resolve(raw.Link, ""),
	}

	if raw.Icon != "" {
		series.PosterURL = s.resolve(raw.Icon, "")
	}

	return series
}
