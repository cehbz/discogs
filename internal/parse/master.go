package parse

import "io"

type Master struct {
	ID          int            `xml:"id,attr"`
	MainRelease int            `xml:"main_release"`
	Title       string         `xml:"title"`
	Year        int            `xml:"year"`
	DataQuality string         `xml:"data_quality"`
	Artists     []ArtistCredit `xml:"artists>artist"`
	Genres      []string       `xml:"genres>genre"`
	Styles      []string       `xml:"styles>style"`
	Videos      []Video        `xml:"videos>video"`
}

func ParseMasters(r io.Reader, fn func(*Master) error) error {
	return streamRecords(r, "master", fn)
}
