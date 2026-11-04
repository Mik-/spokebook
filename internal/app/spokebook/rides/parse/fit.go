package parse

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/profile/untyped/mesgnum"
)

// garminGear is the undocumented message in which Garmin devices (e.g.
// an Edge 1040) write the UUID of the gear chosen for the activity; field 0
// is the same UUID as in Garmin Connect.
const garminGear typedef.MesgNum = 526

// GarminGearPrefix starts the key of a Garmin gear.
const GarminGearPrefix = "gear:"

// isFIT reports whether data starts with a FIT file header.
func isFIT(data []byte) bool {
	return len(data) >= 12 && (data[0] == 12 || data[0] == 14) && string(data[8:12]) == ".FIT"
}

// fit reads the cycling sessions of a FIT file as rides. The bike is the
// Garmin gear chosen for the activity, else the file's bike profile if it
// has exactly one, else the activity profile on the recording device
// (e.g. "Gravel" on an Edge, which writes no bike profile), else the
// device.
func fit(data []byte, loc *time.Location) ([]Ride, error) {
	dec := decoder.New(bytes.NewReader(data))
	var rides []Ride
	for dec.Next() {
		f, err := dec.Decode()
		if err != nil {
			return nil, ErrInvalid
		}
		var (
			fileID   *mesgdef.FileId
			activity *mesgdef.Activity
			sessions []*mesgdef.Session
			profiles []*mesgdef.BikeProfile
			gear     string
		)
		for i := range f.Messages {
			m := &f.Messages[i]
			switch m.Num {
			case mesgnum.FileId:
				fileID = mesgdef.NewFileId(m)
			case mesgnum.Activity:
				activity = mesgdef.NewActivity(m)
			case mesgnum.Session:
				sessions = append(sessions, mesgdef.NewSession(m))
			case mesgnum.BikeProfile:
				profiles = append(profiles, mesgdef.NewBikeProfile(m))
			case garminGear:
				if id, err := uuid.Parse(m.FieldValueByNum(0).String()); err == nil {
					gear = id.String()
				}
			}
		}
		if fileID == nil || fileID.Type != typedef.FileActivity {
			continue
		}
		// FIT times are UTC; the activity tells the offset to local time.
		zone := loc
		if activity != nil && !activity.LocalTimestamp.IsZero() && !activity.Timestamp.IsZero() {
			offset := activity.LocalTimestamp.Sub(activity.Timestamp)
			zone = time.FixedZone("", int(offset.Seconds()))
		}
		for n, s := range sessions {
			if s.Sport != typedef.SportCycling && s.Sport != typedef.SportEBiking {
				continue
			}
			var doubt string
			switch s.SubSport {
			case typedef.SubSportIndoorCycling, typedef.SubSportSpin, typedef.SubSportVirtualActivity:
				doubt = Indoor
			}
			dist := s.TotalDistanceScaled()
			if s.StartTime.IsZero() || math.IsNaN(dist) {
				continue
			}
			key, name := fitGear(fileID, gear, profiles, s.SportProfileName)
			ride := Ride{
				Source: SourceFIT, ExternalID: fitID(fileID, s, n), GearKey: key, GearName: name,
				StartedAt: s.StartTime.UTC(), RiddenOn: localDay(s.StartTime, zone), DistanceM: int64(math.Round(dist)),
				Doubt: doubt,
			}
			ride.EndedAt = ride.StartedAt
			if secs := s.TotalElapsedTimeScaled(); !math.IsNaN(secs) {
				ride.EndedAt = ride.StartedAt.Add(time.Duration(secs * float64(time.Second)))
			}
			rides = append(rides, ride)
		}
	}
	if len(rides) == 0 {
		return nil, ErrNoRide
	}
	return rides, nil
}

// fitID identifies session n of a file by the device's serial number and
// the file's creation time, falling back to the session's start.
func fitID(id *mesgdef.FileId, s *mesgdef.Session, n int) string {
	created := id.TimeCreated
	if created.IsZero() {
		created = s.StartTime
	}
	out := fmt.Sprintf("%d-%d", id.SerialNumber, created.Unix())
	if n > 0 {
		out += "-" + strconv.Itoa(n)
	}
	return out
}

// fitGear returns the key and name of the bike: the Garmin gear, else the
// bike profile if there is exactly one, else the activity profile on the
// device, else the device. The gear has no name in the file, so its name
// is the start of the UUID.
func fitGear(id *mesgdef.FileId, gear string, profiles []*mesgdef.BikeProfile, activity string) (key, name string) {
	if gear != "" {
		return GarminGearPrefix + gear, gear[:8]
	}
	if len(profiles) == 1 && profiles[0].Name != "" {
		return "profile:" + profiles[0].Name, profiles[0].Name
	}
	if id.SerialNumber == 0 || id.SerialNumber == math.MaxUint32 {
		return "", ""
	}
	device := id.ProductName
	if device == "" && id.Manufacturer == typedef.ManufacturerGarmin {
		device = typedef.GarminProduct(id.Product).String()
	}
	if device == "" {
		device = id.Manufacturer.String()
	}
	serial := strconv.FormatUint(uint64(id.SerialNumber), 10)
	if activity != "" {
		return "activity:" + serial + ":" + activity, activity + " (" + device + ")"
	}
	return "device:" + serial, device + " (" + serial + ")"
}
