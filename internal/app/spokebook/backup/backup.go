package backup

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/attachments"
	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

var (
	// ErrInvalid means the file is not a Spokebook backup or its data is
	// inconsistent, e.g. an installation refers to a missing bike.
	ErrInvalid = errors.New("backup: invalid file")
	// ErrVersion means the file was written by a newer Spokebook.
	ErrVersion = errors.New("backup: unsupported version")
)

// Export reads all data of the owner.
func Export(ctx context.Context, st store.Store, ownerID string) (File, error) {
	f := File{Format: Format, Version: Version, ExportedAt: time.Now().UTC()}

	types, err := st.Components().ListTypes(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, t := range types {
		vs, err := st.Components().ListTypeValues(ctx, ownerID, t.ID)
		if err != nil {
			return File{}, err
		}
		f.ComponentTypes = append(f.ComponentTypes, ComponentType{
			ID: t.ID, Name: t.Name, Hidden: t.Hidden, CreatedAt: t.CreatedAt, Values: fromValues(vs),
		})
	}

	bikes, err := st.Bikes().ListBikes(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, b := range bikes {
		fb := Bike{
			ID: b.ID, Name: b.Name, Type: b.Type, Notes: b.Notes, Origin: b.Origin, PurchasedAt: b.PurchasedAt,
			Dealer: b.Dealer, PriceCents: b.PriceCents, Currency: b.Currency, InServiceAt: b.InServiceAt,
			InServiceM: b.InServiceM, Features: b.Features, CreatedAt: b.CreatedAt, RetiredAt: b.RetiredAt,
			RetireReason: b.RetireReason,
		}
		vs, err := st.Bikes().ListBikeValues(ctx, ownerID, b.ID)
		if err != nil {
			return File{}, err
		}
		fb.Values = fromValues(vs)
		readings, err := st.Odometer().ListReadings(ctx, ownerID, b.ID)
		if err != nil {
			return File{}, err
		}
		for _, r := range readings {
			fb.Odometer = append(fb.Odometer, OdometerReading{
				ID: r.ID, RecordedAt: r.RecordedAt, DistanceM: r.DistanceM, Source: r.Source, CreatedAt: r.CreatedAt,
			})
		}
		ms, err := st.Maintenances().ListMaintenances(ctx, ownerID, b.ID)
		if err != nil {
			return File{}, err
		}
		for _, m := range ms {
			if m, err = st.Maintenances().GetMaintenance(ctx, ownerID, m.ID); err != nil {
				return File{}, err
			}
			fb.Maintenances = append(fb.Maintenances, fromMaintenance(m))
		}
		f.Bikes = append(f.Bikes, fb)
	}

	comps, err := st.Components().ListComponents(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, c := range comps {
		fc := Component{
			ID: c.ID, Type: c.Type, Manufacturer: c.Manufacturer, Model: c.Model, PartNumber: c.PartNumber,
			PurchasedAt: c.PurchasedAt, Dealer: c.Dealer, PriceCents: c.PriceCents, Currency: c.Currency,
			MaxDistanceM: c.MaxDistanceM, ReplaceBy: c.ReplaceBy, URL: c.URL, Notes: c.Notes,
			CreatedAt: c.CreatedAt, RetiredAt: c.RetiredAt, RetireReason: c.RetireReason,
		}
		vs, err := st.Components().ListComponentValues(ctx, ownerID, c.ID)
		if err != nil {
			return File{}, err
		}
		fc.Values = fromValues(vs)
		inst, err := st.Components().ListInstallations(ctx, ownerID, c.ID)
		if err != nil {
			return File{}, err
		}
		for _, i := range inst {
			fc.Installations = append(fc.Installations, Installation{
				ID: i.ID, BikeID: i.BikeID, Position: i.Position, InstalledAt: i.InstalledAt, InstalledM: i.InstalledM,
				InstalledAuto: i.InstalledAuto, RemovedAt: i.RemovedAt, RemovedM: i.RemovedM, RemovedAuto: i.RemovedAuto,
				CreatedAt: i.CreatedAt,
			})
		}
		f.Components = append(f.Components, fc)
	}

	steps, err := st.Steps().ListSteps(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, s := range steps {
		fs := Step{
			ID: s.ID, BuiltinID: s.BuiltinID, Hidden: s.Hidden, Position: s.Position, Title: s.Title,
			Instructions: s.Instructions, Phase: s.Phase, Assembly: s.Assembly, Action: s.Action, Module: s.Module,
			Replaces: s.Replaces, Levels: s.Levels, ComponentTypes: s.ComponentTypes, ValueKinds: s.ValueKinds,
			Values: fromValues(s.Values), IntervalMonths: s.Interval.Months, IntervalM: s.Interval.DistanceM,
			CreatedAt: s.CreatedAt,
		}
		for _, fu := range s.FollowUps {
			fs.FollowUps = append(fs.FollowUps, FollowUp{StepID: fu.StepID, Kind: fu.Kind})
		}
		f.Steps = append(f.Steps, fs)
	}
	intervals, err := st.Steps().ListLevelIntervals(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, li := range intervals {
		f.LevelIntervals = append(f.LevelIntervals, LevelInterval{Level: li.Level, Months: li.Months, DistanceM: li.DistanceM})
	}
	rides, err := st.Rides().ListRides(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, r := range rides {
		f.Rides = append(f.Rides, Ride{
			ID: r.ID, BikeID: r.BikeID, Source: r.Source, ExternalID: r.ExternalID, GearKey: r.GearKey,
			BikeByGear: r.BikeByGear, StartedAt: r.StartedAt, EndedAt: r.EndedAt, RiddenOn: r.RiddenOn,
			DistanceM: r.DistanceM, Name: r.Name, Discarded: r.Discarded, CreatedAt: r.CreatedAt,
		})
	}
	gears, err := st.Rides().ListRideGears(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, g := range gears {
		f.RideGears = append(f.RideGears, RideGear{
			ID: g.ID, Source: g.Source, GearKey: g.GearKey, Name: g.Name, BikeID: g.BikeID, CreatedAt: g.CreatedAt,
		})
	}
	items, err := st.Shopping().ListShoppingItems(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	for _, i := range items {
		f.ShoppingItems = append(f.ShoppingItems, ShoppingItem{ID: i.ID, Text: i.Text, BoughtAt: i.BoughtAt, CreatedAt: i.CreatedAt})
	}
	files, err := st.Attachments().ListAttachments(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	links, err := st.Attachments().ListAllAttachmentLinks(ctx, ownerID)
	if err != nil {
		return File{}, err
	}
	byAttachment := make(map[string][]AttachmentLink)
	for _, l := range links {
		byAttachment[l.AttachmentID] = append(byAttachment[l.AttachmentID], AttachmentLink{
			BikeID: l.BikeID, ComponentID: l.ComponentID, Cover: l.Cover,
		})
	}
	for _, a := range files {
		f.Attachments = append(f.Attachments, Attachment{
			ID: a.ID, Name: a.Name, ContentType: a.ContentType, File: "attachments/" + a.ID, CreatedAt: a.CreatedAt,
			Links: byAttachment[a.ID],
		})
	}
	return f, nil
}

// Write exports the owner's data as a ZIP archive to w: the indented JSON
// first, then the attachments one by one.
func Write(ctx context.Context, st store.Store, ownerID string, w io.Writer) error {
	f, err := Export(ctx, st, ownerID)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	jw, err := zw.CreateHeader(&zip.FileHeader{Name: DataFile, Method: zip.Deflate, Modified: f.ExportedAt})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(jw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return err
	}
	for _, a := range f.Attachments {
		_, data, err := st.Attachments().GetAttachmentData(ctx, ownerID, a.ID)
		if err != nil {
			return err
		}
		// Photos and PDFs are compressed already.
		aw, err := zw.CreateHeader(&zip.FileHeader{Name: a.File, Method: zip.Store, Modified: a.CreatedAt})
		if err != nil {
			return err
		}
		if _, err := aw.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Archive is a backup read by Read.
type Archive struct {
	File
	// files are the files in the archive by name; nil for version 1.
	files map[string]*zip.File
}

// Read decodes a backup, a ZIP archive or the JSON file of version 1, and
// checks format and version.
func Read(r io.ReaderAt, size int64) (Archive, error) {
	head := make([]byte, 4)
	if _, err := r.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return Archive{}, ErrInvalid
	}
	if !bytes.Equal(head, []byte("PK\x03\x04")) {
		f, err := decode(io.NewSectionReader(r, 0, size))
		return Archive{File: f}, err
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Archive{}, ErrInvalid
	}
	a := Archive{files: make(map[string]*zip.File, len(zr.File))}
	for _, zf := range zr.File {
		a.files[zf.Name] = zf
	}
	data, ok := a.files[DataFile]
	if !ok {
		return Archive{}, ErrInvalid
	}
	rc, err := data.Open()
	if err != nil {
		return Archive{}, ErrInvalid
	}
	defer func() { _ = rc.Close() }()
	a.File, err = decode(rc)
	return a, err
}

func decode(r io.Reader) (File, error) {
	var f File
	if err := json.NewDecoder(r).Decode(&f); err != nil || f.Format != Format || f.Version < 1 {
		return File{}, ErrInvalid
	}
	if f.Version > Version {
		return File{}, ErrVersion
	}
	return f, nil
}

// open reads a file of the archive, at most limit bytes.
func (a Archive) open(name string, limit int64) ([]byte, error) {
	zf, ok := a.files[name]
	if !ok {
		return nil, fmt.Errorf("%w: missing %s", ErrInvalid, name)
	}
	rc, err := zf.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s too large", ErrInvalid, name)
	}
	return data, nil
}

// Import replaces all data of the owner with the backup in one
// transaction; on error nothing changes. The owner's account stays. Built-in
// types and steps missing from the backup are added afterwards.
func Import(ctx context.Context, st store.Store, ownerID string, a Archive) error {
	err := st.WithTx(ctx, func(tx store.Stores) error {
		if err := tx.Users().DeleteOwnerData(ctx, ownerID); err != nil {
			return err
		}
		if err := restore(ctx, tx, ownerID, a.File); err != nil {
			return err
		}
		return restoreAttachments(ctx, tx, ownerID, a)
	})
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err != nil {
		return err
	}
	if err := components.SeedTypes(ctx, st, ownerID); err != nil {
		return err
	}
	return catalog.SeedSteps(ctx, st, ownerID)
}

// restore writes f into an empty account. Records that a later one refers
// to come first: types, bikes, components, installations, steps, level
// intervals, maintenances.
func restore(ctx context.Context, tx store.Stores, ownerID string, f File) error {
	cs := tx.Components()
	for _, t := range f.ComponentTypes {
		if t.ID == "" {
			return ErrInvalid
		}
		if _, err := cs.CreateType(ctx, store.ComponentType{OwnerID: ownerID, ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt}); err != nil {
			return err
		}
		if t.Hidden {
			if err := cs.HideType(ctx, ownerID, t.ID); err != nil {
				return err
			}
		}
		vs, err := toValues(ownerID, t.Values)
		if err != nil {
			return err
		}
		if err := cs.ReplaceTypeValues(ctx, ownerID, t.ID, vs); err != nil {
			return err
		}
	}

	for _, b := range f.Bikes {
		if b.ID == "" || b.Name == "" {
			return ErrInvalid
		}
		_, err := tx.Bikes().CreateBike(ctx, store.Bike{
			ID: b.ID, OwnerID: ownerID, Name: b.Name, Type: b.Type, Notes: b.Notes, Origin: b.Origin,
			PurchasedAt: b.PurchasedAt, Dealer: b.Dealer, PriceCents: b.PriceCents, Currency: b.Currency,
			InServiceAt: b.InServiceAt, InServiceM: b.InServiceM, Features: b.Features, CreatedAt: b.CreatedAt,
		})
		if err != nil {
			return err
		}
		if b.RetiredAt != nil {
			if err := tx.Bikes().SetBikeRetired(ctx, ownerID, b.ID, b.RetiredAt, b.RetireReason); err != nil {
				return err
			}
		}
		vs, err := toValues(ownerID, b.Values)
		if err != nil {
			return err
		}
		if err := tx.Bikes().ReplaceBikeValues(ctx, ownerID, b.ID, vs); err != nil {
			return err
		}
		for _, r := range b.Odometer {
			if r.ID == "" {
				return ErrInvalid
			}
			_, err := tx.Odometer().AddReading(ctx, store.OdometerReading{
				ID: r.ID, OwnerID: ownerID, BikeID: b.ID, RecordedAt: r.RecordedAt, DistanceM: r.DistanceM,
				Source: r.Source, CreatedAt: r.CreatedAt,
			})
			if err != nil {
				return err
			}
		}
	}

	for _, c := range f.Components {
		if c.ID == "" || c.Type == "" {
			return ErrInvalid
		}
		_, err := cs.CreateComponent(ctx, store.Component{
			ID: c.ID, OwnerID: ownerID, Type: c.Type, Manufacturer: c.Manufacturer, Model: c.Model,
			PartNumber: c.PartNumber, PurchasedAt: c.PurchasedAt, Dealer: c.Dealer, PriceCents: c.PriceCents,
			Currency: c.Currency, MaxDistanceM: c.MaxDistanceM, ReplaceBy: c.ReplaceBy, URL: c.URL, Notes: c.Notes,
			CreatedAt: c.CreatedAt,
		})
		if err != nil {
			return err
		}
		if c.RetiredAt != nil {
			if err := cs.SetComponentRetired(ctx, ownerID, c.ID, c.RetiredAt, c.RetireReason); err != nil {
				return err
			}
		}
		vs, err := toValues(ownerID, c.Values)
		if err != nil {
			return err
		}
		if err := cs.ReplaceComponentValues(ctx, ownerID, c.ID, vs); err != nil {
			return err
		}
		// A component can only have one open installation, so the closed
		// ones are added (and closed) first.
		inst := slices.Clone(c.Installations)
		slices.SortStableFunc(inst, func(a, b Installation) int {
			return cmp.Compare(boolInt(a.RemovedAt == nil), boolInt(b.RemovedAt == nil))
		})
		for _, i := range inst {
			if i.ID == "" || (i.RemovedAt == nil) != (i.RemovedM == nil) {
				return ErrInvalid
			}
			si := store.Installation{
				ID: i.ID, OwnerID: ownerID, ComponentID: c.ID, BikeID: i.BikeID, Position: i.Position,
				InstalledAt: i.InstalledAt, InstalledM: i.InstalledM, InstalledAuto: i.InstalledAuto, CreatedAt: i.CreatedAt,
			}
			if _, err := cs.AddInstallation(ctx, si); err != nil {
				return err
			}
			if i.RemovedAt != nil {
				if err := cs.CloseInstallation(ctx, ownerID, i.ID, *i.RemovedAt, *i.RemovedM, i.RemovedAuto); err != nil {
					return err
				}
			}
		}
	}

	for _, s := range f.Steps {
		iv := store.Interval{Months: s.IntervalMonths, DistanceM: s.IntervalM}
		if s.ID == "" || catalog.CheckInterval(iv) != nil {
			return ErrInvalid
		}
		vs, err := toValues(ownerID, s.Values)
		if err != nil {
			return err
		}
		err = tx.Steps().CreateStep(ctx, store.Step{
			ID: s.ID, OwnerID: ownerID, BuiltinID: s.BuiltinID, Position: s.Position, Title: s.Title,
			Instructions: s.Instructions, Phase: s.Phase, Assembly: s.Assembly, Action: s.Action, Module: s.Module,
			Replaces: s.Replaces, Levels: s.Levels, ComponentTypes: s.ComponentTypes, ValueKinds: s.ValueKinds,
			Values: vs, Interval: iv, CreatedAt: s.CreatedAt,
		})
		if err != nil {
			return err
		}
		if s.Hidden {
			if err := tx.Steps().HideStep(ctx, ownerID, s.ID); err != nil {
				return err
			}
		}
	}
	// Follow-ups once all steps exist.
	for _, s := range f.Steps {
		if len(s.FollowUps) == 0 {
			continue
		}
		step, err := tx.Steps().GetStep(ctx, ownerID, s.ID)
		if err != nil {
			return err
		}
		for _, fu := range s.FollowUps {
			if !slices.Contains(store.FollowUpKinds, fu.Kind) || !slices.ContainsFunc(f.Steps, func(o Step) bool { return o.ID == fu.StepID }) {
				return ErrInvalid
			}
			step.FollowUps = append(step.FollowUps, store.FollowUp{StepID: fu.StepID, Kind: fu.Kind})
		}
		if err := tx.Steps().UpdateStep(ctx, step); err != nil {
			return err
		}
	}

	for _, li := range f.LevelIntervals {
		iv := store.Interval{Months: li.Months, DistanceM: li.DistanceM}
		if !slices.Contains(catalog.IntervalLevels, li.Level) || catalog.CheckInterval(iv) != nil {
			return ErrInvalid
		}
		err := tx.Steps().SetLevelInterval(ctx, ownerID, store.LevelInterval{
			Level: li.Level, Interval: iv,
		})
		if err != nil {
			return err
		}
	}

	for _, r := range f.Rides {
		if r.ID == "" || r.Source == "" || r.ExternalID == "" || r.DistanceM < 0 {
			return ErrInvalid
		}
		_, err := tx.Rides().CreateRide(ctx, store.Ride{
			ID: r.ID, OwnerID: ownerID, BikeID: r.BikeID, Source: r.Source, ExternalID: r.ExternalID,
			GearKey: r.GearKey, BikeByGear: r.BikeByGear, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
			RiddenOn: r.RiddenOn, DistanceM: r.DistanceM, Name: r.Name, Discarded: r.Discarded, CreatedAt: r.CreatedAt,
		})
		if err != nil {
			return err
		}
	}
	for _, g := range f.RideGears {
		if g.ID == "" || g.Source == "" || g.GearKey == "" {
			return ErrInvalid
		}
		_, err := tx.Rides().CreateRideGear(ctx, store.RideGear{
			ID: g.ID, OwnerID: ownerID, Source: g.Source, GearKey: g.GearKey, Name: g.Name, BikeID: g.BikeID,
			CreatedAt: g.CreatedAt,
		})
		if err != nil {
			return err
		}
	}

	for _, i := range f.ShoppingItems {
		if i.ID == "" || strings.TrimSpace(i.Text) == "" {
			return ErrInvalid
		}
		err := tx.Shopping().CreateShoppingItem(ctx, store.ShoppingItem{
			ID: i.ID, OwnerID: ownerID, Text: i.Text, BoughtAt: i.BoughtAt, CreatedAt: i.CreatedAt,
		})
		if err != nil {
			return err
		}
	}

	for _, b := range f.Bikes {
		for _, m := range b.Maintenances {
			if err := restoreMaintenance(ctx, tx.Maintenances(), ownerID, b.ID, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// restoreAttachments adds the attachments after the bikes and components
// they belong to. Images get a new preview, which also checks them.
func restoreAttachments(ctx context.Context, tx store.Stores, ownerID string, a Archive) error {
	for _, fa := range a.Attachments {
		if fa.ID == "" || attachments.CleanName(fa.Name) == "" || len(fa.Links) == 0 {
			return ErrInvalid
		}
		data, err := a.open(fa.File, attachments.MaxFileSize)
		if err != nil {
			return err
		}
		sa := store.Attachment{
			ID: fa.ID, OwnerID: ownerID, Name: attachments.CleanName(fa.Name), ContentType: fa.ContentType,
			Size: int64(len(data)), Data: data, CreatedAt: fa.CreatedAt,
		}
		switch {
		case attachments.IsImage(fa.ContentType):
			if sa.Thumb, err = attachments.Thumbnail(data); err != nil {
				return fmt.Errorf("%w: %w", ErrInvalid, err)
			}
			sa.HasThumb = true
		case fa.ContentType == "application/pdf" && bytes.HasPrefix(data, []byte("%PDF-")):
		default:
			return fmt.Errorf("%w: attachment %s", ErrInvalid, fa.ID)
		}
		if err := tx.Attachments().CreateAttachment(ctx, sa); err != nil {
			return err
		}
		for _, l := range fa.Links {
			if (l.BikeID == "") == (l.ComponentID == "") {
				return ErrInvalid
			}
			err := tx.Attachments().LinkAttachment(ctx, ownerID, store.AttachmentLink{
				AttachmentID: fa.ID, BikeID: l.BikeID, ComponentID: l.ComponentID, Cover: l.Cover && l.BikeID != "",
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreMaintenance(ctx context.Context, ms store.MaintenanceStore, ownerID, bikeID string, m Maintenance) error {
	if m.ID == "" {
		return ErrInvalid
	}
	sm := store.Maintenance{
		ID: m.ID, OwnerID: ownerID, BikeID: bikeID, Title: m.Title, Level: m.Level, PerformedAt: m.PerformedAt,
		DistanceM: m.DistanceM, Notes: m.Notes, CreatedAt: m.CreatedAt,
	}
	for _, s := range m.Steps {
		if s.ID == "" {
			return ErrInvalid
		}
		step := store.MaintenanceStep{
			ID: s.ID, Position: s.Position, CatalogStepID: s.CatalogStepID, BuiltinID: s.BuiltinID, Title: s.Title,
			Instructions: s.Instructions, Phase: s.Phase, Assembly: s.Assembly, Action: s.Action, Module: s.Module,
			Hints: s.Hints, OriginID: s.OriginID, Added: s.Added,
		}
		plain := make([]Value, len(s.Values))
		for i, v := range s.Values {
			plain[i] = v.Value
		}
		vs, err := toValues(ownerID, plain)
		if err != nil {
			return err
		}
		for i, v := range s.Values {
			step.Values = append(step.Values, store.MaintenanceValue{
				Value: vs[i], ComponentID: v.ComponentID, ComponentName: v.ComponentName,
				ComponentType: v.ComponentType, ComponentPosition: v.ComponentPosition,
			})
		}
		sm.Steps = append(sm.Steps, step)
	}
	if err := ms.CreateMaintenance(ctx, sm); err != nil {
		return err
	}
	// Results are not part of a new maintenance, so they are set afterwards.
	for _, s := range m.Steps {
		if s.Done || s.Note != "" {
			if err := ms.SetStepResult(ctx, ownerID, s.ID, s.Done, s.Note); err != nil {
				return err
			}
		}
		if s.Dismissed {
			if err := ms.SetStepDismissed(ctx, ownerID, s.ID, true); err != nil {
				return err
			}
		}
		for _, v := range s.Values {
			if v.Measured != nil {
				if err := ms.SetMeasured(ctx, ownerID, v.ID, v.Measured); err != nil {
					return err
				}
			}
		}
	}
	if m.CompletedAt != nil {
		return ms.SetMaintenanceCompleted(ctx, ownerID, m.ID, m.CompletedAt)
	}
	return nil
}

func fromValues(vs []store.Value) []Value {
	out := make([]Value, 0, len(vs))
	for _, v := range vs {
		out = append(out, fromValue(v))
	}
	return out
}

func fromValue(v store.Value) Value {
	return Value{ID: v.ID, Key: v.Key, Kind: v.Kind, Label: v.Label, From: v.From, To: v.To, Unit: v.Unit, Hint: v.Hint}
}

// toValues checks the values like a form would and keeps their IDs and
// keys (the values of a maintenance may share a key, e.g. two brake pads).
func toValues(ownerID string, vs []Value) ([]store.Value, error) {
	in := make([]store.Value, 0, len(vs))
	for _, v := range vs {
		if v.ID == "" {
			return nil, ErrInvalid
		}
		in = append(in, store.Value{Key: v.Key, Kind: v.Kind, Label: v.Label, From: v.From, To: v.To, Unit: v.Unit, Hint: v.Hint})
	}
	out, err := values.Prepare(ownerID, in)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	for i := range out {
		out[i].ID = vs[i].ID
		if vs[i].Key != "" {
			out[i].Key = vs[i].Key
		}
	}
	return out, nil
}

func fromMaintenance(m store.Maintenance) Maintenance {
	fm := Maintenance{
		ID: m.ID, Title: m.Title, Level: m.Level, PerformedAt: m.PerformedAt, DistanceM: m.DistanceM,
		Notes: m.Notes, CompletedAt: m.CompletedAt, CreatedAt: m.CreatedAt,
	}
	for _, s := range m.Steps {
		fs := MaintenanceStep{
			ID: s.ID, Position: s.Position, CatalogStepID: s.CatalogStepID, BuiltinID: s.BuiltinID, Title: s.Title,
			Instructions: s.Instructions, Phase: s.Phase, Assembly: s.Assembly, Action: s.Action, Module: s.Module,
			Hints: s.Hints, OriginID: s.OriginID, Added: s.Added, Done: s.Done, Dismissed: s.Dismissed, Note: s.Note,
		}
		for _, v := range s.Values {
			fs.Values = append(fs.Values, MaintenanceValue{
				Value: fromValue(v.Value), ComponentID: v.ComponentID, ComponentName: v.ComponentName,
				ComponentType: v.ComponentType, ComponentPosition: v.ComponentPosition, Measured: v.Measured,
			})
		}
		fm.Steps = append(fm.Steps, fs)
	}
	return fm
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
