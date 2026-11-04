// Package parse reads rides from FIT, GPX and TCX files. Only what a ride
// needs is read: start, end, distance and, if the file tells, the bike.
package parse

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path"
	"strings"
	"time"
)

// Sources of rides read from files.
const (
	SourceFIT = "fit"
	SourceGPX = "gpx"
	SourceTCX = "tcx"
)

var (
	// ErrUnknownFormat means the content is neither FIT, GPX nor TCX.
	ErrUnknownFormat = errors.New("parse: unknown file format")
	// ErrNoTime means the file has no timestamps, e.g. a planned route.
	ErrNoTime = errors.New("parse: no timestamps")
	// ErrNoRide means the file has no cycling activity.
	ErrNoRide = errors.New("parse: no ride")
	// ErrInvalid means the file is damaged.
	ErrInvalid = errors.New("parse: invalid file")
)

// maxSize limits a file after decompression.
const maxSize = 64 << 20

// Ride is a ride read from a file.
type Ride struct {
	Source     string
	ExternalID string
	// GearKey identifies the bike in the source, GearName is its name;
	// both empty if the file doesn't tell.
	GearKey   string
	GearName  string
	StartedAt time.Time
	EndedAt   time.Time
	// RiddenOn is the local date of the start at 00:00 UTC.
	RiddenOn  time.Time
	DistanceM int64
	Name      string
	// Doubt says why the ride may not belong to a bike's odometer, so an
	// import doesn't take it unasked: Indoor or OtherSport; empty if
	// it does.
	Doubt string
}

// Doubts about a ride.
const (
	// Indoor is a ride on a trainer or a virtual ride.
	Indoor = "indoor"
	// OtherSport means the file doesn't tell that it is a ride.
	OtherSport = "other_sport"
)

// File reads the rides of a file; the format is taken from the content,
// gzip compressed files are unpacked. name is the file name, the ride's
// name if the file has none. loc is the time zone for files that only
// have UTC times.
func File(name string, r io.Reader, loc *time.Location) ([]Ride, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, ErrInvalid
		}
		if data, err = io.ReadAll(io.LimitReader(zr, maxSize+1)); err != nil {
			return nil, ErrInvalid
		}
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	if len(data) > maxSize {
		return nil, ErrInvalid
	}
	name = strings.TrimSuffix(path.Base(name), path.Ext(name))
	var rides []Ride
	switch {
	case isFIT(data):
		rides, err = fit(data, loc)
	case isXML(data, "<gpx"):
		rides, err = gpx(data, loc)
	case isXML(data, "<TrainingCenterDatabase"):
		rides, err = tcx(data, loc)
	default:
		return nil, ErrUnknownFormat
	}
	for i := range rides {
		if rides[i].Name == "" {
			rides[i].Name = name
		}
	}
	return rides, err
}

// isXML reports whether data is XML with the root element tag.
func isXML(data []byte, tag string) bool {
	head := data[:min(len(data), 4096)]
	return bytes.Contains(head, []byte(tag))
}

// localDay returns the date of t in loc at 00:00 UTC.
func localDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
