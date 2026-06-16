// Package spotify is the library behind the spot command: the HTTP client,
// OAuth token management, and typed data models for the Spotify Web API.
//
// Authentication uses the Client Credentials flow. Set SPOTIFY_CLIENT_ID and
// SPOTIFY_CLIENT_SECRET. The token is cached in memory and auto-refreshed.
// No user authorization is needed; this grants access to Spotify's public
// catalog (tracks, albums, artists, playlists, new releases).
package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Host is the Spotify API hostname.
const Host = "spotify.com"

// APIBase is the root for all Spotify Web API requests.
const APIBase = "https://api.spotify.com/v1"

// TokenURL is the Spotify OAuth token endpoint.
const TokenURL = "https://accounts.spotify.com/api/token"

// DefaultUserAgent identifies the CLI to the Spotify API.
const DefaultUserAgent = "spot/dev (+https://github.com/tamnd/spotify-cli)"

// ErrNotFound is returned when Spotify returns a 404 for a resource.
var ErrNotFound = errors.New("not found")

// ErrNoCredentials is returned when ClientID or ClientSecret is empty.
var ErrNoCredentials = errors.New("spotify: SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET are required; create an app at https://developer.spotify.com/dashboard")

// Config holds constructor parameters for Client.
type Config struct {
	ClientID     string
	ClientSecret string
	UserAgent    string
	Rate         time.Duration
	Retries      int
	Timeout      time.Duration
}

// DefaultConfig returns sensible defaults for the Spotify client.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// tokenResponse is the JSON from the Spotify token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Client is a rate-limited, token-managing HTTP client for the Spotify API.
type Client struct {
	cfg      Config
	http     *http.Client
	mu       sync.Mutex
	token    string
	tokenExp time.Time
	last     time.Time
}

// NewClient returns a Client configured with cfg.
// It does not make any network calls until the first API method is called.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// bearerToken returns a valid bearer token, fetching or refreshing if needed.
func (c *Client) bearerToken(ctx context.Context) (string, error) {
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return "", ErrNoCredentials
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Refresh if within 60 seconds of expiry.
	if c.token != "" && time.Until(c.tokenExp) > 60*time.Second {
		return c.token, nil
	}
	return c.fetchToken(ctx)
}

// fetchToken fetches a new access token using Client Credentials flow.
// Must be called with mu held.
func (c *Client) fetchToken(ctx context.Context) (string, error) {
	creds := base64.StdEncoding.EncodeToString(
		[]byte(c.cfg.ClientID + ":" + c.cfg.ClientSecret))

	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("token read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token fetch http %d: %s", resp.StatusCode, string(b))
	}
	var tr tokenResponse
	if err := json.Unmarshal(b, &tr); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}
	c.token = tr.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}

// get fetches an API path with the bearer token, pacing and retries.
func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	token, err := c.bearerToken(ctx)
	if err != nil {
		return nil, err
	}

	rawURL := APIBase + path
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 500 * time.Millisecond
			if wait > 5*time.Second {
				wait = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		body, retry, err := c.do(ctx, rawURL, token)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", path, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL, token string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return b, false, nil
}

// pace sleeps until at least Rate has elapsed since the last request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

// Search searches the Spotify catalog.
func (c *Client) Search(ctx context.Context, query, itemType string, market string, limit int) ([]SearchResult, error) {
	q := url.Values{
		"q":      {query},
		"type":   {itemType},
		"limit":  {fmt.Sprintf("%d", limit)},
		"market": {market},
	}
	b, err := c.get(ctx, "/search", q)
	if err != nil {
		return nil, err
	}
	return parseSearchResults(b, itemType)
}

// GetTrack fetches a single track by ID.
func (c *Client) GetTrack(ctx context.Context, id string) (*Track, error) {
	b, err := c.get(ctx, "/tracks/"+id, nil)
	if err != nil {
		return nil, err
	}
	return parseTrack(b)
}

// GetAlbum fetches a single album by ID.
func (c *Client) GetAlbum(ctx context.Context, id string) (*Album, error) {
	b, err := c.get(ctx, "/albums/"+id, nil)
	if err != nil {
		return nil, err
	}
	return parseAlbum(b)
}

// GetArtist fetches a single artist by ID.
func (c *Client) GetArtist(ctx context.Context, id string) (*Artist, error) {
	b, err := c.get(ctx, "/artists/"+id, nil)
	if err != nil {
		return nil, err
	}
	return parseArtist(b)
}

// GetNewReleases fetches new album releases for the given country.
func (c *Client) GetNewReleases(ctx context.Context, country string, limit int) ([]Release, error) {
	q := url.Values{
		"country": {country},
		"limit":   {fmt.Sprintf("%d", limit)},
	}
	b, err := c.get(ctx, "/browse/new-releases", q)
	if err != nil {
		return nil, err
	}
	return parseNewReleases(b)
}

// ExtractID extracts the Spotify ID from a URL or returns the input as-is.
// Spotify URLs look like: https://open.spotify.com/track/<id>
func ExtractID(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		u, err := url.Parse(input)
		if err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 {
				return parts[len(parts)-1]
			}
		}
	}
	return input
}

// FormatDuration formats a duration in milliseconds as "M:SS".
func FormatDuration(ms int) string {
	total := ms / 1000
	m := total / 60
	s := total % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

// --- raw API response shapes ---

type rawTrack struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Artists  []rawArtist `json:"artists"`
	Album    rawAlbum    `json:"album"`
	DurMs    int         `json:"duration_ms"`
	Pop      int         `json:"popularity"`
	Explicit bool        `json:"explicit"`
	Preview  string      `json:"preview_url"`
	ExtURLs  struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

type rawArtist struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Genres  []string `json:"genres"`
	Followers struct {
		Total int `json:"total"`
	} `json:"followers"`
	Pop     int `json:"popularity"`
	ExtURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

type rawAlbum struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Artists     []rawArtist `json:"artists"`
	AlbumType   string      `json:"album_type"`
	TotalTracks int         `json:"total_tracks"`
	ReleaseDate string      `json:"release_date"`
	Pop         int         `json:"popularity"`
	Label       string      `json:"label"`
	ExtURLs     struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

// --- parsers ---

func parseTrack(b []byte) (*Track, error) {
	var r rawTrack
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode track: %w", err)
	}
	return trackFromRaw(r), nil
}

func trackFromRaw(r rawTrack) *Track {
	artists := make([]string, len(r.Artists))
	artist := ""
	for i, a := range r.Artists {
		artists[i] = a.Name
		if i == 0 {
			artist = a.Name
		}
	}
	return &Track{
		ID:          r.ID,
		Name:        r.Name,
		Artist:      artist,
		Artists:     artists,
		Album:       r.Album.Name,
		AlbumID:     r.Album.ID,
		DurationMs:  r.DurMs,
		Duration:    FormatDuration(r.DurMs),
		Popularity:  r.Pop,
		Explicit:    r.Explicit,
		PreviewURL:  r.Preview,
		URL:         r.ExtURLs.Spotify,
		ReleaseDate: r.Album.ReleaseDate,
	}
}

func parseAlbum(b []byte) (*Album, error) {
	var r rawAlbum
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode album: %w", err)
	}
	return albumFromRaw(r), nil
}

func albumFromRaw(r rawAlbum) *Album {
	artists := make([]string, len(r.Artists))
	artist := ""
	for i, a := range r.Artists {
		artists[i] = a.Name
		if i == 0 {
			artist = a.Name
		}
	}
	return &Album{
		ID:          r.ID,
		Name:        r.Name,
		Artist:      artist,
		Artists:     artists,
		AlbumType:   r.AlbumType,
		TotalTracks: r.TotalTracks,
		ReleaseDate: r.ReleaseDate,
		Popularity:  r.Pop,
		URL:         r.ExtURLs.Spotify,
		Label:       r.Label,
	}
}

func parseArtist(b []byte) (*Artist, error) {
	var r rawArtist
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode artist: %w", err)
	}
	return artistFromRaw(r), nil
}

func artistFromRaw(r rawArtist) *Artist {
	genres := r.Genres
	if genres == nil {
		genres = []string{}
	}
	return &Artist{
		ID:         r.ID,
		Name:       r.Name,
		Genres:     genres,
		Followers:  r.Followers.Total,
		Popularity: r.Pop,
		URL:        r.ExtURLs.Spotify,
	}
}

func parseSearchResults(b []byte, itemType string) ([]SearchResult, error) {
	var raw struct {
		Tracks  *struct{ Items []rawTrack  } `json:"tracks"`
		Albums  *struct{ Items []rawAlbum  } `json:"albums"`
		Artists *struct{ Items []rawArtist } `json:"artists"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}
	var out []SearchResult
	switch itemType {
	case "track":
		if raw.Tracks != nil {
			for _, t := range raw.Tracks.Items {
				artist := ""
				album := ""
				if len(t.Artists) > 0 {
					artist = t.Artists[0].Name
				}
				if t.Album.Name != "" {
					album = t.Album.Name
				}
				out = append(out, SearchResult{
					ID:     t.ID,
					Name:   t.Name,
					Type:   "track",
					Artist: artist,
					Album:  album,
					URL:    t.ExtURLs.Spotify,
				})
			}
		}
	case "album":
		if raw.Albums != nil {
			for _, a := range raw.Albums.Items {
				artist := ""
				if len(a.Artists) > 0 {
					artist = a.Artists[0].Name
				}
				out = append(out, SearchResult{
					ID:     a.ID,
					Name:   a.Name,
					Type:   "album",
					Artist: artist,
					URL:    a.ExtURLs.Spotify,
				})
			}
		}
	case "artist":
		if raw.Artists != nil {
			for _, a := range raw.Artists.Items {
				out = append(out, SearchResult{
					ID:   a.ID,
					Name: a.Name,
					Type: "artist",
					URL:  a.ExtURLs.Spotify,
				})
			}
		}
	}
	return out, nil
}

func parseNewReleases(b []byte) ([]Release, error) {
	var raw struct {
		Albums struct {
			Items []rawAlbum `json:"items"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("decode new releases: %w", err)
	}
	out := make([]Release, len(raw.Albums.Items))
	for i, a := range raw.Albums.Items {
		artists := make([]string, len(a.Artists))
		artist := ""
		for j, ar := range a.Artists {
			artists[j] = ar.Name
			if j == 0 {
				artist = ar.Name
			}
		}
		out[i] = Release{
			ID:          a.ID,
			Name:        a.Name,
			Artist:      artist,
			Artists:     artists,
			ReleaseDate: a.ReleaseDate,
			TotalTracks: a.TotalTracks,
			AlbumType:   a.AlbumType,
			URL:         a.ExtURLs.Spotify,
		}
	}
	return out, nil
}
