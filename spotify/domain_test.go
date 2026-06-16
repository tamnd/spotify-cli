package spotify

import (
	"testing"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "spotify" {
		t.Errorf("Scheme = %q, want spotify", info.Scheme)
	}
	if len(info.Hosts) == 0 {
		t.Error("Hosts is empty")
	}
	if info.Identity.Binary != "spot" {
		t.Errorf("Identity.Binary = %q, want spot", info.Identity.Binary)
	}
}

func TestClassifySpotifyURL(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"https://open.spotify.com/track/4iV5W9uYEdYUVa79Axb7Rh", "track", "4iV5W9uYEdYUVa79Axb7Rh"},
		{"https://open.spotify.com/album/6XhjNHCyCDyyGJRM5mg40G", "album", "6XhjNHCyCDyyGJRM5mg40G"},
		{"https://open.spotify.com/artist/0gxyHStUsqpMadRV0Di1Qt", "artist", "0gxyHStUsqpMadRV0Di1Qt"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyBareID(t *testing.T) {
	typ, id, err := Domain{}.Classify("4iV5W9uYEdYUVa79Axb7Rh")
	if err != nil {
		t.Fatalf("Classify bare ID: %v", err)
	}
	if typ != "track" {
		t.Errorf("type = %q, want track", typ)
	}
	if id != "4iV5W9uYEdYUVa79Axb7Rh" {
		t.Errorf("id = %q", id)
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify empty string should return error")
	}
}

func TestLocateTrack(t *testing.T) {
	got, err := Domain{}.Locate("track", "4iV5W9uYEdYUVa79Axb7Rh")
	want := "https://open.spotify.com/track/4iV5W9uYEdYUVa79Axb7Rh"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateArtist(t *testing.T) {
	got, err := Domain{}.Locate("artist", "0gxyHStUsqpMadRV0Di1Qt")
	want := "https://open.spotify.com/artist/0gxyHStUsqpMadRV0Di1Qt"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return error")
	}
}
