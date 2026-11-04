package backup_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/attachments"
	"github.com/Mik-/spokebook/internal/app/spokebook/backup"
	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

func ptr(f float64) *float64 { return &f }

func ptr64(i int64) *int64 { return &i }

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrAt(s string) *time.Time {
	t := at(s)
	return &t
}

// open returns an empty store with the built-in types and steps of owner.
func open(t *testing.T, owner string) store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := components.SeedTypes(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	return st
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// fill adds one record of every kind, with optional fields set.
func fill(t *testing.T, st store.Store, owner string) {
	t.Helper()
	ctx := context.Background()
	cs := st.Components()
	must(t, cs.HideType(ctx, owner, "kickstand"))
	must(t, cs.RenameType(ctx, owner, "chain", "Kette"))
	_, err := cs.CreateType(ctx, store.ComponentType{OwnerID: owner, ID: "0190-type", Name: "Dynamo", CreatedAt: at("2026-01-01T00:00:00Z")})
	must(t, err)
	must(t, cs.ReplaceTypeValues(ctx, owner, "chain", []store.Value{
		{ID: "tv1", Key: "wear", Kind: store.KindLimit, Label: "Wear", To: ptr(0.5), Unit: "%"},
	}))

	_, err = st.Bikes().CreateBike(ctx, store.Bike{
		ID: "b1", OwnerID: owner, Name: "Gravel", Type: "gravel", Notes: "n", Origin: "self_built",
		PurchasedAt: ptrAt("2025-03-01T00:00:00Z"), Dealer: "Shop", PriceCents: ptr64(250000), Currency: "EUR",
		InServiceAt: ptrAt("2025-03-02T00:00:00Z"), InServiceM: ptr64(0), Features: []string{"carbon", "tubeless"},
		CreatedAt: at("2025-03-01T10:00:00Z"),
	})
	must(t, err)
	_, err = st.Bikes().CreateBike(ctx, store.Bike{ID: "b2", OwnerID: owner, Name: "Old", CreatedAt: at("2020-01-01T00:00:00Z")})
	must(t, err)
	must(t, st.Bikes().SetBikeRetired(ctx, owner, "b2", ptrAt("2024-01-01T00:00:00Z"), "sold"))
	must(t, st.Bikes().ReplaceBikeValues(ctx, owner, "b1", []store.Value{
		{ID: "bv1", Key: "saddle_height", Kind: store.KindInfo, Label: "Saddle height", From: ptr(745), To: ptr(745), Unit: "mm", Hint: "BB to top"},
	}))
	_, err = st.Odometer().AddReading(ctx, store.OdometerReading{
		ID: "r1", OwnerID: owner, BikeID: "b1", RecordedAt: at("2026-05-01T00:00:00Z"), DistanceM: 1234000,
		Source: store.SourceManual, CreatedAt: at("2026-05-01T08:00:00Z"),
	})
	must(t, err)

	_, err = cs.CreateComponent(ctx, store.Component{
		ID: "c1", OwnerID: owner, Type: "chain", Manufacturer: "KMC", Model: "X11", PartNumber: "123",
		PurchasedAt: ptrAt("2025-03-01T00:00:00Z"), Dealer: "Shop", PriceCents: ptr64(3000), Currency: "EUR",
		MaxDistanceM: ptr64(3000000), ReplaceBy: ptrAt("2027-01-01T00:00:00Z"), URL: "https://example.com",
		Notes: "n", CreatedAt: at("2025-03-01T10:00:00Z"),
	})
	must(t, err)
	must(t, cs.ReplaceComponentValues(ctx, owner, "c1", []store.Value{
		{ID: "cv1", Key: "wear", Kind: store.KindLimit, Label: "Wear", To: ptr(0.75), Unit: "%"},
	}))
	_, err = cs.AddInstallation(ctx, store.Installation{
		ID: "i1", OwnerID: owner, ComponentID: "c1", BikeID: "b2", InstalledAt: at("2023-01-01T00:00:00Z"),
		CreatedAt: at("2023-01-01T00:00:00Z"),
	})
	must(t, err)
	must(t, cs.CloseInstallation(ctx, owner, "i1", at("2023-06-01T00:00:00Z"), 500000, false))
	_, err = cs.AddInstallation(ctx, store.Installation{
		ID: "i2", OwnerID: owner, ComponentID: "c1", BikeID: "b1", Position: "rear", InstalledAt: at("2025-03-02T00:00:00Z"),
		InstalledM: 0, CreatedAt: at("2025-03-02T00:00:00Z"),
	})
	must(t, err)
	_, err = cs.CreateComponent(ctx, store.Component{ID: "c2", OwnerID: owner, Type: "0190-type", CreatedAt: at("2025-01-01T00:00:00Z")})
	must(t, err)
	must(t, cs.SetComponentRetired(ctx, owner, "c2", ptrAt("2025-02-01T00:00:00Z"), "broken"))

	steps, err := st.Steps().ListSteps(ctx, owner)
	must(t, err)
	must(t, st.Steps().HideStep(ctx, owner, steps[0].ID))
	must(t, st.WithTx(ctx, func(tx store.Stores) error {
		return tx.Steps().CreateStep(ctx, store.Step{
			ID: "s1", OwnerID: owner, Position: 999, Title: "Check dynamo", Instructions: "Spin it",
			Phase: steps[1].Phase, Action: "check", Levels: []string{"monthly"}, ComponentTypes: []string{"0190-type"},
			ValueKinds: []string{store.KindTorque},
			Values:     []store.Value{{ID: "sv1", Key: "bolt", Kind: store.KindTorque, Label: "Bolt", From: ptr(5), To: ptr(5), Unit: "Nm", Hint: "paste"}},
			Interval:   store.Interval{DistanceM: ptr64(500000)},
			CreatedAt:  at("2026-01-01T00:00:00Z"),
		})
	}))
	must(t, st.Steps().SetLevelInterval(ctx, owner, store.LevelInterval{Level: "minor", Interval: store.Interval{Months: ptr64(4)}}))
	must(t, st.Steps().SetLevelInterval(ctx, owner, store.LevelInterval{Level: "monthly"}))
	must(t, st.Shopping().CreateShoppingItem(ctx, store.ShoppingItem{ID: "sh1", OwnerID: owner, Text: "Chain oil", CreatedAt: at("2026-05-01T09:00:00Z")}))
	must(t, st.Shopping().CreateShoppingItem(ctx, store.ShoppingItem{
		ID: "sh2", OwnerID: owner, Text: "Sealant", BoughtAt: ptrAt("2026-05-02T00:00:00Z"), CreatedAt: at("2026-05-01T09:01:00Z"),
	}))

	must(t, st.WithTx(ctx, func(tx store.Stores) error {
		return tx.Maintenances().CreateMaintenance(ctx, store.Maintenance{
			ID: "m1", OwnerID: owner, BikeID: "b1", Level: "monthly", PerformedAt: at("2026-05-01T00:00:00Z"),
			DistanceM: 1234000, Notes: "fine", CreatedAt: at("2026-05-01T09:00:00Z"),
			Steps: []store.MaintenanceStep{{
				ID: "ms1", Position: 3, CatalogStepID: "s1", Title: "Check chain", Phase: steps[1].Phase,
				Action: "measure", Module: "carbon", Hints: []string{"carbon_paste"},
				Values: []store.MaintenanceValue{
					{Value: store.Value{ID: "mv1", Key: "wear", Kind: store.KindLimit, Label: "Wear", To: ptr(0.75), Unit: "%"}, ComponentID: "c1", ComponentName: "KMC X11", ComponentType: "chain", ComponentPosition: "rear"},
					{Value: store.Value{ID: "mv2", Key: "wear", Kind: store.KindLimit, Label: "Wear", To: ptr(0.75), Unit: "%"}, ComponentID: "c2", ComponentType: "chain"},
				},
			}, {ID: "ms2", Position: 4, Title: "Clean", Phase: steps[1].Phase, Added: true}, {ID: "ms3", Position: 5, Title: "Replace chain", Phase: steps[1].Phase, OriginID: "ms1"}},
		})
	}))
	must(t, st.Maintenances().SetStepResult(ctx, owner, "ms1", true, "ok"))
	must(t, st.Maintenances().SetMeasured(ctx, owner, "mv1", ptr(0.5)))
	must(t, st.Maintenances().SetStepDismissed(ctx, owner, "ms3", true))
	must(t, st.Maintenances().SetMaintenanceCompleted(ctx, owner, "m1", ptrAt("2026-05-01T12:00:00Z")))

	// A photo of the bike and a receipt for two components.
	var photo bytes.Buffer
	must(t, jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil))
	thumb, err := attachments.Thumbnail(photo.Bytes())
	must(t, err)
	as := st.Attachments()
	must(t, as.CreateAttachment(ctx, store.Attachment{
		ID: "a1", OwnerID: owner, Name: "bike.jpg", ContentType: "image/jpeg", Data: photo.Bytes(), Thumb: thumb,
		CreatedAt: at("2026-01-03T00:00:00Z"),
	}))
	must(t, as.CreateAttachment(ctx, store.Attachment{
		ID: "a2", OwnerID: owner, Name: "receipt.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4"),
		CreatedAt: at("2026-01-04T00:00:00Z"),
	}))
	for _, l := range []store.AttachmentLink{
		{AttachmentID: "a1", BikeID: "b1", Cover: true}, {AttachmentID: "a2", ComponentID: "c1"}, {AttachmentID: "a2", ComponentID: "c2"},
	} {
		must(t, as.LinkAttachment(ctx, owner, l))
	}
}

func export(t *testing.T, st store.Store, owner string) backup.File {
	t.Helper()
	f, err := backup.Export(context.Background(), st, owner)
	must(t, err)
	f.ExportedAt = time.Time{}
	return f
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := open(t, "alice")
	fill(t, src, "alice")
	var buf bytes.Buffer
	must(t, backup.Write(ctx, src, "alice", &buf))

	// The target has an account and data of its own, which is replaced.
	dst := open(t, "bob")
	must(t, dst.Users().CreateUser(ctx, store.User{ID: "bob", Name: "bob", PasswordHash: "h", CreatedAt: time.Now()}))
	must(t, dst.Users().CreatePasskey(ctx, store.Passkey{ID: "p1", UserID: "bob", CredentialID: []byte{1}, Name: "key", Credential: []byte{2}, CreatedAt: time.Now()}))
	_, err := dst.Bikes().CreateBike(ctx, store.Bike{ID: "other", OwnerID: "bob", Name: "Other", CreatedAt: time.Now()})
	must(t, err)

	f, err := backup.Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	must(t, err)
	must(t, backup.Import(ctx, dst, "bob", f))

	want, got := export(t, src, "alice"), export(t, dst, "bob")
	if !slices.ContainsFunc(want.Steps, func(s backup.Step) bool { return len(s.FollowUps) > 0 }) {
		t.Error("no follow-ups exported")
	}
	if !slices.ContainsFunc(want.Bikes, func(b backup.Bike) bool { return len(b.Values) > 0 }) {
		t.Error("no bike values exported")
	}
	if !reflect.DeepEqual(want, got) {
		w, _ := json.MarshalIndent(want, "", " ")
		g, _ := json.MarshalIndent(got, "", " ")
		t.Errorf("round trip differs:\nwant %s\ngot  %s", w, g)
	}
	if ct, data, err := dst.Attachments().GetAttachmentData(ctx, "bob", "a2"); err != nil || ct != "application/pdf" || string(data) != "%PDF-1.4" {
		t.Errorf("receipt %q %q %v", ct, data, err)
	}
	if _, err := dst.Attachments().GetAttachmentThumb(ctx, "bob", "a1"); err != nil {
		t.Errorf("photo preview: %v", err)
	}
	if _, err := dst.Bikes().GetBike(ctx, "bob", "other"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("old bike kept: %v", err)
	}
	if u, err := dst.Users().GetUser(ctx, "bob"); err != nil || u.PasswordHash != "h" {
		t.Errorf("account changed: %+v %v", u, err)
	}
	if keys, err := dst.Users().ListPasskeys(ctx, "bob"); err != nil || len(keys) != 1 {
		t.Errorf("passkeys changed: %d %v", len(keys), err)
	}
}

func TestImportAddsMissingBuiltins(t *testing.T) {
	ctx := context.Background()
	st := open(t, "o")
	must(t, backup.Import(ctx, st, "o", backup.Archive{File: backup.File{Format: backup.Format, Version: 1}}))
	types, err := st.Components().ListTypes(ctx, "o")
	must(t, err)
	steps, err := st.Steps().ListSteps(ctx, "o")
	must(t, err)
	if len(types) != len(components.BuiltinTypes) || len(steps) != len(catalog.Builtins) {
		t.Errorf("types %d, steps %d", len(types), len(steps))
	}
}

func TestInvalid(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		json string
		want error
	}{
		"not json":           {`{`, backup.ErrInvalid},
		"other format":       {`{"format":"x","version":1}`, backup.ErrInvalid},
		"newer":              {`{"format":"spokebook-backup","version":99}`, backup.ErrVersion},
		"missing bike":       {`{"format":"spokebook-backup","version":1,"components":[{"id":"c","type":"chain","installations":[{"id":"i","bike_id":"nope"}]}]}`, backup.ErrInvalid},
		"bad value":          {`{"format":"spokebook-backup","version":1,"components":[{"id":"c","type":"chain","values":[{"id":"v","label":"x"}]}]}`, backup.ErrInvalid},
		"duplicate":          {`{"format":"spokebook-backup","version":1,"bikes":[{"id":"b","name":"x"},{"id":"b","name":"y"}]}`, backup.ErrInvalid},
		"missing follow-up":  {`{"format":"spokebook-backup","version":2,"steps":[{"id":"s","phase":"inspection","follow_ups":[{"step_id":"nope","kind":"finding"}]}]}`, backup.ErrInvalid},
		"bad follow-up kind": {`{"format":"spokebook-backup","version":2,"steps":[{"id":"s","phase":"inspection"},{"id":"t","phase":"inspection","follow_ups":[{"step_id":"s","kind":"later"}]}]}`, backup.ErrInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			st := open(t, "o")
			_, err := st.Bikes().CreateBike(ctx, store.Bike{ID: "keep", OwnerID: "o", Name: "Keep", CreatedAt: time.Now()})
			must(t, err)
			f, err := backup.Read(strings.NewReader(tc.json), int64(len(tc.json)))
			if err == nil {
				err = backup.Import(ctx, st, "o", f)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if _, err := st.Bikes().GetBike(ctx, "o", "keep"); err != nil {
				t.Errorf("data lost after failed import: %v", err)
			}
		})
	}
}

// archive builds a ZIP backup from name/content pairs.
func archive(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < len(files); i += 2 {
		w, err := zw.Create(files[i])
		must(t, err)
		_, err = w.Write([]byte(files[i+1]))
		must(t, err)
	}
	must(t, zw.Close())
	return buf.Bytes()
}

func TestInvalidArchive(t *testing.T) {
	ctx := context.Background()
	const head = `{"format":"spokebook-backup","version":2,"bikes":[{"id":"b","name":"x"}],"attachments":[`
	for name, tc := range map[string]struct {
		zip  []byte
		want error
	}{
		"no data":      {archive(t, "other.json", "{}"), backup.ErrInvalid},
		"broken":       {[]byte("PK\x03\x04 broken"), backup.ErrInvalid},
		"missing file": {archive(t, backup.DataFile, head+`{"id":"a","name":"a.pdf","content_type":"application/pdf","file":"attachments/a","links":[{"bike_id":"b"}]}]}`), backup.ErrInvalid},
		"not a pdf":    {archive(t, backup.DataFile, head+`{"id":"a","name":"a.pdf","content_type":"application/pdf","file":"f","links":[{"bike_id":"b"}]}]}`, "f", "<html>"), backup.ErrInvalid},
		"bad image":    {archive(t, backup.DataFile, head+`{"id":"a","name":"a.jpg","content_type":"image/jpeg","file":"f","links":[{"bike_id":"b"}]}]}`, "f", "x"), backup.ErrInvalid},
		"no link":      {archive(t, backup.DataFile, head+`{"id":"a","name":"a.pdf","content_type":"application/pdf","file":"f","links":[]}]}`, "f", "%PDF-"), backup.ErrInvalid},
		"unknown bike": {archive(t, backup.DataFile, head+`{"id":"a","name":"a.pdf","content_type":"application/pdf","file":"f","links":[{"bike_id":"x"}]}]}`, "f", "%PDF-"), backup.ErrInvalid},
		"ok":           {archive(t, backup.DataFile, head+`{"id":"a","name":"a.pdf","content_type":"application/pdf","file":"f","links":[{"bike_id":"b"}]}]}`, "f", "%PDF-"), nil},
	} {
		t.Run(name, func(t *testing.T) {
			st := open(t, "o")
			f, err := backup.Read(bytes.NewReader(tc.zip), int64(len(tc.zip)))
			if err == nil {
				err = backup.Import(ctx, st, "o", f)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
