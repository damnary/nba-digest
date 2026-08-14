package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/platform/retry"
)

const (
	defaultSiteBase = "https://site.api.espn.com/apis/site/v2/sports/basketball"
	defaultCoreBase = "https://sports.core.api.espn.com/v2/sports/basketball/leagues"
	defaultPageSize = 25
)

type Client struct {
	http     *http.Client
	siteBase string
	coreBase string
	pageSize int
	now      func() time.Time

	mu    sync.Mutex
	codes map[core.League]map[string]core.TeamCode
}

type Option func(*Client)

func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.http = c }
}

func WithBaseURLs(site, core string) Option {
	return func(cl *Client) {
		cl.siteBase = site
		cl.coreBase = core
	}
}

func WithPageSize(n int) Option {
	return func(cl *Client) { cl.pageSize = n }
}

func WithClock(now func() time.Time) Option {
	return func(cl *Client) { cl.now = now }
}

func New(opts ...Option) *Client {
	c := &Client{
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &retry.Transport{Policy: retry.Default},
		},
		siteBase: defaultSiteBase,
		coreBase: defaultCoreBase,
		pageSize: defaultPageSize,
		now:      time.Now,
		codes:    make(map[core.League]map[string]core.TeamCode),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Name() string { return "espn" }

func (c *Client) Teams(ctx context.Context, league core.League) ([]core.ExternalTeam, error) {
	var resp teamsResponse
	u := fmt.Sprintf("%s/%s/teams", c.siteBase, league)
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return nil, fmt.Errorf("espn teams: %w", err)
	}

	teams := mapTeams(league, resp)
	if len(teams) == 0 {
		return nil, fmt.Errorf("espn teams: empty roster for %s", league)
	}
	c.cacheCodes(league, teams)
	return teams, nil
}

func (c *Client) Games(ctx context.Context, league core.League, day core.Day) ([]core.Game, error) {
	u := fmt.Sprintf("%s/%s/scoreboard?dates=%04d%02d%02d",
		c.siteBase, league, day.Year, int(day.Month), day.Day)

	var resp scoreboardResponse
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return nil, fmt.Errorf("espn scoreboard: %w", err)
	}
	return c.mapScoreboard(league, resp, c.now())
}

func (c *Client) Plays(ctx context.Context, game core.Game, cursor string) (core.PlayFeed, error) {
	ext, err := externalID(game.ID)
	if err != nil {
		return core.PlayFeed{}, err
	}

	codes, err := c.teamCodes(ctx, game.League)
	if err != nil {
		return core.PlayFeed{}, err
	}

	page := 1
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil && n > 0 {
			page = n
		}
	}

	feed := core.PlayFeed{Cursor: cursor}
	for {
		u := fmt.Sprintf("%s/%s/events/%s/competitions/%s/plays?limit=%d&page=%d",
			c.coreBase, game.League, ext, ext, c.pageSize, page)

		var resp playsResponse
		if err := c.getJSON(ctx, u, &resp); err != nil {
			return core.PlayFeed{}, fmt.Errorf("espn plays page %d: %w", page, err)
		}

		feed.Plays = append(feed.Plays, mapPlays(resp, codes)...)
		feed.Cursor = strconv.Itoa(page)

		if resp.PageCount <= page {
			break
		}
		page++
	}
	return feed, nil
}

func (c *Client) BoxScore(ctx context.Context, game core.Game) (core.GameStats, error) {
	ext, err := externalID(game.ID)
	if err != nil {
		return core.GameStats{}, err
	}

	u := fmt.Sprintf("%s/%s/summary?event=%s", c.siteBase, game.League, url.QueryEscape(ext))

	var resp summaryResponse
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return core.GameStats{}, fmt.Errorf("espn summary: %w", err)
	}
	return mapBoxScore(game, resp), nil
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) teamCodes(ctx context.Context, league core.League) (map[string]core.TeamCode, error) {
	c.mu.Lock()
	cached, ok := c.codes[league]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}

	if _, err := c.Teams(ctx, league); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.codes[league], nil
}

func (c *Client) cacheCodes(league core.League, teams []core.ExternalTeam) {
	codes := make(map[string]core.TeamCode, len(teams))
	for _, t := range teams {
		codes[t.ExternalID] = t.Team.Code
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.codes[league] = codes
}
