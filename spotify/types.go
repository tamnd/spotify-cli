package spotify

// Track is a Spotify track record.
type Track struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	AlbumID     string   `json:"album_id"`
	DurationMs  int      `json:"duration_ms"`
	Duration    string   `json:"duration"`
	Popularity  int      `json:"popularity"`
	Explicit    bool     `json:"explicit"`
	PreviewURL  string   `json:"preview_url,omitempty"`
	URL         string   `json:"url"`
	ReleaseDate string   `json:"release_date,omitempty"`
}

// Album is a Spotify album record.
type Album struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	AlbumType   string   `json:"album_type"`
	TotalTracks int      `json:"total_tracks"`
	ReleaseDate string   `json:"release_date"`
	Popularity  int      `json:"popularity"`
	URL         string   `json:"url"`
	Label       string   `json:"label,omitempty"`
}

// Artist is a Spotify artist record.
type Artist struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Genres     []string `json:"genres"`
	Followers  int      `json:"followers"`
	Popularity int      `json:"popularity"`
	URL        string   `json:"url"`
}

// SearchResult is one item from a Spotify search response.
type SearchResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	URL    string `json:"url"`
}

// Release is one item from the new releases endpoint.
type Release struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	ReleaseDate string   `json:"release_date"`
	TotalTracks int      `json:"total_tracks"`
	AlbumType   string   `json:"album_type"`
	URL         string   `json:"url"`
}
