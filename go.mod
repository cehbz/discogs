module github.com/charles-haynes/discogs

go 1.13

replace github.com/charles-haynes/gazelle => ../gazelle

replace github.com/charles-haynes/whatapi => ../whatapi

require (
	github.com/anacrolix/tagflag v0.0.0-20180803105420-3a8ff5428f76
	github.com/anacrolix/torrent v1.5.2
	github.com/antchfx/htmlquery v1.0.0
	github.com/bradfitz/iter v0.0.0-20190303215204-33e6a9893b0c
	github.com/charles-haynes/gazelle v0.1.0
	github.com/charles-haynes/html2bbcode v0.0.0-20191010140350-63d1fbb9d7d7
	github.com/charles-haynes/munkres v0.0.0-20191008174651-55d467190535
	github.com/charles-haynes/strsim v0.0.0-20191011181331-ef9ead4980ee
	github.com/charles-haynes/whatapi v0.0.14
	github.com/edsrzf/mmap-go v1.0.0
	github.com/irlndts/go-discogs v0.0.0-20181211134731-618b88263431
	github.com/jmoiron/sqlx v1.2.0
	github.com/mattn/go-sqlite3 v1.11.0
	github.com/pkg/errors v0.8.1
	github.com/xrash/smetrics v0.0.0-20170218160415-a3153f7040e9
	golang.org/x/net v0.38.0
	gopkg.in/yaml.v2 v2.2.4
)
