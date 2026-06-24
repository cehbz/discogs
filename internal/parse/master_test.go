package parse

import (
	"strings"
	"testing"
)

func TestParseMasters(t *testing.T) {
	const xml = `<masters>
<master id="100"><main_release>555</main_release><title>Selected Ambient Works</title>
  <year>1992</year><data_quality>Correct</data_quality>
  <artists><artist><id>1</id><name>Aphex Twin</name><anv/><join/><role/></artist></artists>
  <genres><genre>Electronic</genre></genres>
  <styles><style>Ambient</style><style>IDM</style></styles>
  <videos><video src="https://y.t/x" duration="362"><title>Xtal</title><description>d</description></video></videos>
</master>
</masters>`
	var got []*Master
	if err := ParseMasters(strings.NewReader(xml), func(m *Master) error { got = append(got, m); return nil }); err != nil {
		t.Fatalf("ParseMasters: %v", err)
	}
	m := got[0]
	if m.ID != 100 || m.MainRelease != 555 || m.Year != 1992 || m.Title != "Selected Ambient Works" {
		t.Fatalf("core: %+v", m)
	}
	if len(m.Artists) != 1 || m.Artists[0].ID != 1 {
		t.Fatalf("artists: %+v", m.Artists)
	}
	if len(m.Styles) != 2 || m.Styles[1] != "IDM" {
		t.Fatalf("styles: %v", m.Styles)
	}
	if len(m.Videos) != 1 || m.Videos[0].Duration != 362 || m.Videos[0].Title != "Xtal" {
		t.Fatalf("videos: %+v", m.Videos)
	}
}
