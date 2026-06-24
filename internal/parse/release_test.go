package parse

import (
	"strings"
	"testing"
)

const releaseXML = `<releases>
<release id="3" status="Accepted">
  <artists><artist><id>3</id><name>Josh Wink</name><anv/><join/><role/><tracks/></artist></artists>
  <title>Profound Sounds Vol. 1</title>
  <labels><label name="Ruffhouse Records" catno="CK 63628" id="6"/></labels>
  <extraartists><artist><id>99</id><name>Some Engineer</name><role>Mastered By</role></artist></extraartists>
  <formats><format name="CD" qty="1" text="">
    <descriptions><description>Compilation</description><description>Mixed</description></descriptions>
  </format></formats>
  <genres><genre>Electronic</genre></genres>
  <styles><style>Techno</style></styles>
  <country>US</country><released>1999-07-13</released><data_quality>Correct</data_quality>
  <master_id is_main_release="false">66526</master_id>
  <tracklist><track><position>1</position><title>Untitled 8</title><duration>7:00</duration></track></tracklist>
  <identifiers><identifier type="Barcode" value="074646362822" description="text"/></identifiers>
  <companies><company><id>5</id><name>PolyGram</name><catno>x</catno><entity_type>13</entity_type><entity_type_name>Manufactured By</entity_type_name></company></companies>
  <videos><video src="https://y.t/v" duration="420"><title>V</title><description>d</description></video></videos>
</release>
</releases>`

func TestParseReleases(t *testing.T) {
	var got []*Release
	if err := ParseReleases(strings.NewReader(releaseXML), func(r *Release) error { got = append(got, r); return nil }); err != nil {
		t.Fatalf("ParseReleases: %v", err)
	}
	r := got[0]
	if r.ID != 3 || r.Status != "Accepted" || r.Title != "Profound Sounds Vol. 1" || r.Country != "US" || r.Released != "1999-07-13" {
		t.Fatalf("core: %+v", r)
	}
	if r.Master == nil || r.Master.ID != 66526 || r.Master.IsMainRelease {
		t.Fatalf("master ref: %+v", r.Master)
	}
	if len(r.Artists) != 1 || r.Artists[0].ID != 3 {
		t.Fatalf("artists: %+v", r.Artists)
	}
	if len(r.ExtraArtists) != 1 || r.ExtraArtists[0].Role != "Mastered By" {
		t.Fatalf("extraartists: %+v", r.ExtraArtists)
	}
	if len(r.Labels) != 1 || r.Labels[0].Catno != "CK 63628" || r.Labels[0].ID != 6 {
		t.Fatalf("labels: %+v", r.Labels)
	}
	if len(r.Formats) != 1 || len(r.Formats[0].Descriptions) != 2 || r.Formats[0].Name != "CD" {
		t.Fatalf("formats: %+v", r.Formats)
	}
	if len(r.Identifiers) != 1 || r.Identifiers[0].Value != "074646362822" {
		t.Fatalf("identifiers: %+v", r.Identifiers)
	}
	if len(r.Companies) != 1 || r.Companies[0].EntityTypeName != "Manufactured By" {
		t.Fatalf("companies: %+v", r.Companies)
	}
	if len(r.Tracklist) != 1 || r.Tracklist[0].Title != "Untitled 8" {
		t.Fatalf("tracklist: %+v", r.Tracklist)
	}
}
