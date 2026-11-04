package parse

import (
	"bytes"
	"encoding/xml"
	"math"
	"strings"
	"time"
)

type gpxFile struct {
	Metadata struct {
		Name string `xml:"name"`
	} `xml:"metadata"`
	Tracks []struct {
		Name     string `xml:"name"`
		Type     string `xml:"type"`
		Segments []struct {
			Points []struct {
				Lat  float64 `xml:"lat,attr"`
				Lon  float64 `xml:"lon,attr"`
				Time string  `xml:"time"`
			} `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
}

// gpx reads a GPX file as one ride: the distance between the track points
// of all segments, from the first to the last time. A track type (e.g.
// "cycling" from Garmin) that is another sport skips the file; one that is
// unknown makes the ride doubtful.
func gpx(data []byte, loc *time.Location) ([]Ride, error) {
	var f gpxFile
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return nil, ErrInvalid
	}
	ride := Ride{Source: SourceGPX, Name: strings.TrimSpace(f.Metadata.Name)}
	var dist float64
	for _, trk := range f.Tracks {
		switch gpxSport(trk.Type) {
		case "other":
			return nil, ErrNoRide
		case "unknown":
			ride.Doubt = OtherSport
		}
		if ride.Name == "" {
			ride.Name = strings.TrimSpace(trk.Name)
		}
		for _, seg := range trk.Segments {
			for i, p := range seg.Points {
				if i > 0 {
					prev := seg.Points[i-1]
					dist += haversine(prev.Lat, prev.Lon, p.Lat, p.Lon)
				}
				t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(p.Time))
				if err != nil {
					continue
				}
				if ride.StartedAt.IsZero() || t.Before(ride.StartedAt) {
					ride.StartedAt = t
				}
				if t.After(ride.EndedAt) {
					ride.EndedAt = t
				}
			}
		}
	}
	if ride.StartedAt.IsZero() {
		return nil, ErrNoTime
	}
	ride.StartedAt, ride.EndedAt = ride.StartedAt.UTC(), ride.EndedAt.UTC()
	ride.ExternalID = ride.StartedAt.Format(time.RFC3339)
	ride.RiddenOn = localDay(ride.StartedAt, loc)
	ride.DistanceM = int64(math.Round(dist))
	return []Ride{ride}, nil
}

// gpxSport classifies a GPX track type as "" (cycling or no type),
// "other" (another sport) or "unknown". Apps write different words, e.g.
// "cycling", "road_biking", "Ride", "running", "hiking".
func gpxSport(typ string) string {
	t := strings.ToLower(strings.TrimSpace(typ))
	switch {
	case t == "":
		return ""
	case containsAny(t, "cycl", "bik", "ride", "velo", "rad"):
		return ""
	case containsAny(t, "run", "hik", "walk", "swim", "ski", "row", "paddl", "kayak", "canoe", "trek", "lauf", "wander"):
		return "other"
	}
	return "unknown"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// haversine returns the distance in meters between two points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371008.8
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}

type tcxFile struct {
	Activities []struct {
		Sport string `xml:"Sport,attr"`
		ID    string `xml:"Id"`
		Notes string `xml:"Notes"`
		Laps  []struct {
			StartTime string  `xml:"StartTime,attr"`
			Seconds   float64 `xml:"TotalTimeSeconds"`
			Distance  float64 `xml:"DistanceMeters"`
		} `xml:"Lap"`
	} `xml:"Activities>Activity"`
}

// tcx reads the activities of a TCX file except runs, one ride each: the
// laps' distance, from the start for the laps' time. A sport other than
// "Biking" (TCX only knows "Other" besides) makes the ride doubtful.
func tcx(data []byte, loc *time.Location) ([]Ride, error) {
	var f tcxFile
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return nil, ErrInvalid
	}
	var rides []Ride
	for _, a := range f.Activities {
		if strings.EqualFold(a.Sport, "Running") {
			continue
		}
		start, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(a.ID))
		if err != nil {
			return nil, ErrNoTime
		}
		ride := Ride{
			Source: SourceTCX, ExternalID: start.UTC().Format(time.RFC3339), StartedAt: start.UTC(),
			RiddenOn: localDay(start, loc), Name: strings.TrimSpace(a.Notes),
		}
		if !strings.EqualFold(a.Sport, "Biking") {
			ride.Doubt = OtherSport
		}
		var dist, secs float64
		end := start
		for _, l := range a.Laps {
			dist += l.Distance
			secs += l.Seconds
			if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(l.StartTime)); err == nil {
				end = maxTime(end, t.Add(time.Duration(l.Seconds*float64(time.Second))))
			}
		}
		ride.EndedAt = maxTime(end, start.Add(time.Duration(secs*float64(time.Second)))).UTC()
		ride.DistanceM = int64(math.Round(dist))
		rides = append(rides, ride)
	}
	if len(rides) == 0 {
		return nil, ErrNoRide
	}
	return rides, nil
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
