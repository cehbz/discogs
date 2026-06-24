package parse

import "io"

type MasterRef struct {
	ID            int  `xml:",chardata"`
	IsMainRelease bool `xml:"is_main_release,attr"`
}

type ReleaseLabel struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Catno string `xml:"catno,attr"`
}

type Format struct {
	Name         string   `xml:"name,attr"`
	Qty          string   `xml:"qty,attr"`
	Text         string   `xml:"text,attr"`
	Descriptions []string `xml:"descriptions>description"`
}

type Identifier struct {
	Type        string `xml:"type,attr"`
	Value       string `xml:"value,attr"`
	Description string `xml:"description,attr"`
}

type Company struct {
	ID             int    `xml:"id"`
	Name           string `xml:"name"`
	Catno          string `xml:"catno"`
	EntityType     string `xml:"entity_type"`
	EntityTypeName string `xml:"entity_type_name"`
}

type Track struct {
	Position     string         `xml:"position"`
	Title        string         `xml:"title"`
	Duration     string         `xml:"duration"`
	Artists      []ArtistCredit `xml:"artists>artist"`
	ExtraArtists []ArtistCredit `xml:"extraartists>artist"`
	SubTracks    []Track        `xml:"sub_tracks>track"`
}

type Release struct {
	ID           int            `xml:"id,attr"`
	Status       string         `xml:"status,attr"`
	Title        string         `xml:"title"`
	Country      string         `xml:"country"`
	Released     string         `xml:"released"`
	Notes        string         `xml:"notes"`
	DataQuality  string         `xml:"data_quality"`
	Master       *MasterRef     `xml:"master_id"`
	Artists      []ArtistCredit `xml:"artists>artist"`
	ExtraArtists []ArtistCredit `xml:"extraartists>artist"`
	Labels       []ReleaseLabel `xml:"labels>label"`
	Formats      []Format       `xml:"formats>format"`
	Genres       []string       `xml:"genres>genre"`
	Styles       []string       `xml:"styles>style"`
	Identifiers  []Identifier   `xml:"identifiers>identifier"`
	Companies    []Company      `xml:"companies>company"`
	Videos       []Video        `xml:"videos>video"`
	Tracklist    []Track        `xml:"tracklist>track"`
}

func ParseReleases(r io.Reader, fn func(*Release) error) error {
	return streamRecords(r, "release", fn)
}
