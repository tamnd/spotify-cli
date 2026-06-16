package spotify

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes spotify as a kit Domain. A multi-domain host enables it
// with a single blank import:
//
//	import _ "github.com/tamnd/spotify-cli/spotify"
//
// The same Domain builds the standalone spot binary via cli.NewApp.
func init() { kit.Register(Domain{}) }

// Domain is the Spotify driver. It carries no state; the per-run client is
// built by the factory Register hands to kit.
type Domain struct{}

// Info describes the scheme and identity that the binary inherits.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "spotify",
		Hosts:  []string{Host, "open.spotify.com"},
		Identity: kit.Identity{
			Binary: "spot",
			Short:  "Read Spotify music catalog data",
			Long: `spot turns the Spotify Web API into a fast, scriptable command line.

Requires SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET. Get free credentials at
https://developer.spotify.com/dashboard (no premium subscription needed).

Quick start:
  spot search "taylor swift"               search tracks
  spot search "beethoven" --type album     search albums
  spot track 4iV5W9uYEdYUVa79Axb7Rh       fetch a track
  spot album 6XhjNHCyCDyyGJRM5mg40G       fetch an album
  spot artist 0gxyHStUsqpMadRV0Di1Qt      fetch an artist
  spot new                                 new releases`,
			Site: Host,
			Repo: "https://github.com/tamnd/spotify-cli",
		},
	}
}

// Register installs the client factory and all operations onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "catalog",
		Summary: "Search the Spotify catalog",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, searchCatalog)

	kit.Handle(app, kit.OpMeta{
		Name:     "track",
		Group:    "catalog",
		Single:   true,
		Resolver: true,
		URIType:  "track",
		Summary:  "Fetch a track by ID or URL",
		Args:     []kit.Arg{{Name: "id", Help: "Spotify track ID or URL"}},
	}, getTrack)

	kit.Handle(app, kit.OpMeta{
		Name:     "album",
		Group:    "catalog",
		Single:   true,
		Resolver: true,
		URIType:  "album",
		Summary:  "Fetch an album by ID or URL",
		Args:     []kit.Arg{{Name: "id", Help: "Spotify album ID or URL"}},
	}, getAlbum)

	kit.Handle(app, kit.OpMeta{
		Name:     "artist",
		Group:    "catalog",
		Single:   true,
		Resolver: true,
		URIType:  "artist",
		Summary:  "Fetch an artist by ID or URL",
		Args:     []kit.Arg{{Name: "id", Help: "Spotify artist ID or URL"}},
	}, getArtist)

	kit.Handle(app, kit.OpMeta{
		Name:    "new",
		Group:   "browse",
		Summary: "List new album releases",
	}, getNewReleases)
}

// newClient builds a Client from the kit Config and environment.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	c.ClientID = os.Getenv("SPOTIFY_CLIENT_ID")
	c.ClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- input structs ---

type searchInput struct {
	Query   string  `kit:"arg" help:"search query"`
	Type    string  `kit:"flag" help:"content type: track, album, artist" default:"track"`
	Country string  `kit:"flag" help:"ISO 3166-1 alpha-2 market" default:"US"`
	Limit   int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client  *Client `kit:"inject"`
}

type trackInput struct {
	ID     string  `kit:"arg" help:"Spotify track ID or URL"`
	Client *Client `kit:"inject"`
}

type albumInput struct {
	ID     string  `kit:"arg" help:"Spotify album ID or URL"`
	Client *Client `kit:"inject"`
}

type artistInput struct {
	ID     string  `kit:"arg" help:"Spotify artist ID or URL"`
	Client *Client `kit:"inject"`
}

type newReleasesInput struct {
	Country string  `kit:"flag" help:"ISO 3166-1 alpha-2 country code" default:"US"`
	Limit   int     `kit:"flag,inherit" help:"max releases" default:"20"`
	Client  *Client `kit:"inject"`
}

// --- handlers ---

func searchCatalog(ctx context.Context, in searchInput, emit func(SearchResult) error) error {
	t := in.Type
	if t == "" {
		t = "track"
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	country := in.Country
	if country == "" {
		country = "US"
	}
	results, err := in.Client.Search(ctx, in.Query, t, country, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func getTrack(ctx context.Context, in trackInput, emit func(*Track) error) error {
	id := ExtractID(in.ID)
	t, err := in.Client.GetTrack(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	return emit(t)
}

func getAlbum(ctx context.Context, in albumInput, emit func(*Album) error) error {
	id := ExtractID(in.ID)
	a, err := in.Client.GetAlbum(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	return emit(a)
}

func getArtist(ctx context.Context, in artistInput, emit func(*Artist) error) error {
	id := ExtractID(in.ID)
	a, err := in.Client.GetArtist(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	return emit(a)
}

func getNewReleases(ctx context.Context, in newReleasesInput, emit func(Release) error) error {
	country := in.Country
	if country == "" {
		country = "US"
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	releases, err := in.Client.GetNewReleases(ctx, country, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range releases {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

// Classify turns an ID or Spotify URL into (uriType, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("spotify: empty input")
	}
	// Spotify URL: https://open.spotify.com/<type>/<id>
	if strings.Contains(input, "open.spotify.com") {
		parts := strings.Split(strings.Trim(input, "/"), "/")
		if len(parts) >= 2 {
			uriType = parts[len(parts)-2]
			id = parts[len(parts)-1]
			return uriType, id, nil
		}
	}
	// Bare ID: assume track (most common use case).
	return "track", input, nil
}

// Locate returns the canonical URL for a (uriType, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "track", "album", "artist", "playlist":
		return "https://open.spotify.com/" + uriType + "/" + id, nil
	default:
		return "", errs.Usage("spotify has no resource type %q", uriType)
	}
}

// --- helpers ---

// mapErr maps library sentinel errors to kit error kinds.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return errs.NotFound("%s", err.Error())
	}
	if errors.Is(err, ErrNoCredentials) {
		return errs.NeedAuth("%s", err.Error())
	}
	return err
}
