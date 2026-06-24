package parse

import "io"

type Artist struct {
	ID             int         `xml:"id"`
	Name           string      `xml:"name"`
	RealName       string      `xml:"realname"`
	Profile        string      `xml:"profile"`
	DataQuality    string      `xml:"data_quality"`
	NameVariations []string    `xml:"namevariations>name"`
	URLs           []string    `xml:"urls>url"`
	Aliases        []ArtistRef `xml:"aliases>name"`
	Members        []ArtistRef `xml:"members>name"`
	Groups         []ArtistRef `xml:"groups>name"`
}

func ParseArtists(r io.Reader, fn func(*Artist) error) error {
	return streamRecords(r, "artist", fn)
}
