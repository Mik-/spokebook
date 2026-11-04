package parse_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/spokebook/spokebook/internal/app/spokebook/rides/parse"
)

var berlin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
	return loc
}()

const gpxFile = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><name>Evening ride</name><trkseg>
    <trkpt lat="52.0" lon="13.0"><time>2026-06-30T22:30:00Z</time></trkpt>
    <trkpt lat="52.01" lon="13.0"><time>2026-06-30T22:35:00Z</time></trkpt>
    <trkpt lat="52.02" lon="13.0"><time>2026-06-30T22:40:00Z</time></trkpt>
  </trkseg></trk>
</gpx>`

func TestGPX(t *testing.T) {
	rides, err := parse.File("evening.gpx", strings.NewReader(gpxFile), berlin)
	if err != nil {
		t.Fatal(err)
	}
	r := rides[0]
	// 0.02° latitude are about 2224 m; 22:30 UTC is already July 1st in Berlin.
	if len(rides) != 1 || r.Source != parse.SourceGPX || r.DistanceM != 2224 || r.Name != "Evening ride" ||
		r.ExternalID != "2026-06-30T22:30:00Z" || !r.RiddenOn.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) ||
		r.EndedAt.Sub(r.StartedAt) != 10*time.Minute {
		t.Fatalf("ride = %+v", r)
	}
}

func TestGPXSport(t *testing.T) {
	for typ, want := range map[string]string{"": "", "cycling": "", "road_biking": "", "Ride": "", "1": parse.OtherSport, "running": "skip", "hiking": "skip"} {
		file := strings.Replace(gpxFile, "<name>Evening ride</name>", "<name>x</name><type>"+typ+"</type>", 1)
		rides, err := parse.File("x.gpx", strings.NewReader(file), berlin)
		switch {
		case want == "skip" && !errors.Is(err, parse.ErrNoRide):
			t.Errorf("%q: err = %v", typ, err)
		case want != "skip" && (err != nil || rides[0].Doubt != want):
			t.Errorf("%q: rides = %+v, err = %v", typ, rides, err)
		}
	}
}

func TestGPXWithoutTime(t *testing.T) {
	route := `<gpx><trk><trkseg><trkpt lat="52.0" lon="13.0"/><trkpt lat="52.1" lon="13.0"/></trkseg></trk></gpx>`
	if _, err := parse.File("route.gpx", strings.NewReader(route), berlin); !errors.Is(err, parse.ErrNoTime) {
		t.Fatalf("err = %v", err)
	}
}

func TestTCX(t *testing.T) {
	tcx := `<?xml version="1.0"?>
<TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2">
  <Activities>
    <Activity Sport="Biking">
      <Id>2026-05-01T08:00:00Z</Id>
      <Lap StartTime="2026-05-01T08:00:00Z"><TotalTimeSeconds>1800</TotalTimeSeconds><DistanceMeters>10000.4</DistanceMeters></Lap>
      <Lap StartTime="2026-05-01T08:30:00Z"><TotalTimeSeconds>1800</TotalTimeSeconds><DistanceMeters>12000</DistanceMeters></Lap>
    </Activity>
    <Activity Sport="Running"><Id>2026-05-02T08:00:00Z</Id></Activity>
    <Activity Sport="Other"><Id>2026-05-03T08:00:00Z</Id></Activity>
  </Activities>
</TrainingCenterDatabase>`
	rides, err := parse.File("morning.tcx", strings.NewReader(tcx), berlin)
	if err != nil {
		t.Fatal(err)
	}
	if len(rides) != 2 || rides[0].DistanceM != 22000 || rides[0].Name != "morning" || rides[0].Doubt != "" ||
		rides[1].Doubt != parse.OtherSport ||
		!rides[0].EndedAt.Equal(time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("rides = %+v", rides)
	}
}

// fitFile returns a FIT file of a ride with the activity profile and bike
// profiles.
func fitFile(t *testing.T, sport typedef.Sport, activity string, profiles ...string) []byte {
	t.Helper()
	return fitFileWith(t, sport, activity, nil, profiles...)
}

// fitFileWith is fitFile with more messages.
func fitFileWith(t *testing.T, sport typedef.Sport, activity string, more []proto.Message, profiles ...string) []byte {
	t.Helper()
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	fileID := mesgdef.NewFileId(nil).SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerGarmin).
		SetProduct(uint16(typedef.GarminProductEdge530)).SetSerialNumber(3412345).SetTimeCreated(start)
	msgs := []proto.Message{fileID.ToMesg(nil)}
	for _, p := range profiles {
		msgs = append(msgs, mesgdef.NewBikeProfile(nil).SetName(p).ToMesg(nil))
	}
	msgs = append(msgs, more...)
	msgs = append(msgs,
		mesgdef.NewSession(nil).SetStartTime(start).SetSport(sport).SetSportProfileName(activity).SetTotalDistance(4_567_890).
			SetTotalElapsedTime(7_200_000).SetTimestamp(start.Add(2*time.Hour)).ToMesg(nil),
		mesgdef.NewActivity(nil).SetTimestamp(start.Add(2*time.Hour)).
			SetLocalTimestamp(start.Add(4*time.Hour)).ToMesg(nil),
	)
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&proto.FIT{Messages: msgs}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFIT(t *testing.T) {
	rides, err := parse.File("ride.fit", bytes.NewReader(fitFile(t, typedef.SportCycling, "Gravel", "Grizl")), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	r := rides[0]
	if len(rides) != 1 || r.Source != parse.SourceFIT || r.DistanceM != 45679 || r.GearKey != "profile:Grizl" ||
		r.GearName != "Grizl" || r.EndedAt.Sub(r.StartedAt) != 2*time.Hour || r.Name != "ride" ||
		r.ExternalID == "" {
		t.Fatalf("ride = %+v", r)
	}
}

func TestFITDevice(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(fitFile(t, typedef.SportEBiking, ""))
	_ = zw.Close()
	rides, err := parse.File("ride.fit.gz", &gz, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if r := rides[0]; r.GearKey != "device:3412345" || !strings.HasSuffix(r.GearName, " (3412345)") || r.Name != "ride" {
		t.Fatalf("ride = %+v", r)
	}
}

// An Edge writes no bike profile; its activity profiles tell the bikes
// apart.
func TestFITActivityProfile(t *testing.T) {
	for _, activity := range []string{"Gravel", "Arbeitsweg"} {
		rides, err := parse.File("ride.fit", bytes.NewReader(fitFile(t, typedef.SportCycling, activity)), time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		if r := rides[0]; r.GearKey != "activity:3412345:"+activity || r.GearName != activity+" (edge_530)" {
			t.Fatalf("ride = %+v", r)
		}
	}
}

// Garmin writes the UUID of the gear chosen for the activity; it wins
// over bike and activity profile.
func TestFITGarminGear(t *testing.T) {
	const id = "07411423-ebc0-4657-a4a0-d68864fed343"
	gear := proto.Message{Num: 526, Fields: []proto.Field{{
		FieldBase: &proto.FieldBase{Name: "unknown", Num: 0, BaseType: basetype.String, Scale: 1},
		Value:     proto.String(id),
	}}}
	rides, err := parse.File("ride.fit", bytes.NewReader(fitFileWith(t, typedef.SportCycling, "Gravel", []proto.Message{gear}, "Grizl")), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if r := rides[0]; r.GearKey != "gear:"+id || r.GearName != "07411423" {
		t.Fatalf("ride = %+v", r)
	}
}

func TestFITIndoor(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	msgs := []proto.Message{
		mesgdef.NewFileId(nil).SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerZwift).SetSerialNumber(1).
			SetTimeCreated(start).ToMesg(nil),
		mesgdef.NewSession(nil).SetStartTime(start).SetSport(typedef.SportCycling).SetSubSport(typedef.SubSportVirtualActivity).
			SetTotalDistance(3_000_000).SetTotalElapsedTime(3_600_000).SetTimestamp(start.Add(time.Hour)).ToMesg(nil),
	}
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&proto.FIT{Messages: msgs}); err != nil {
		t.Fatal(err)
	}
	rides, err := parse.File("zwift.fit", &buf, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if rides[0].Doubt != parse.Indoor {
		t.Fatalf("ride = %+v", rides[0])
	}
}

func TestFITNotCycling(t *testing.T) {
	_, err := parse.File("run.fit", bytes.NewReader(fitFile(t, typedef.SportRunning, "")), time.UTC)
	if !errors.Is(err, parse.ErrNoRide) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownFormat(t *testing.T) {
	if _, err := parse.File("x.txt", strings.NewReader("hello"), time.UTC); !errors.Is(err, parse.ErrUnknownFormat) {
		t.Fatalf("err = %v", err)
	}
}
