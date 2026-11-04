// Package backup exports all data of an owner to a ZIP archive and
// restores it. The archive holds the data as JSON (DataFile) and the
// attachments as files. Accounts, sessions and passkeys are not part of a
// backup.
package backup

import "time"

// Format identifies a Spokebook backup file; Version is the version of the
// file format written by Export. Import reads every version up to Version.
// Version 1 was the JSON file alone, version 2 put it into a ZIP archive
// with the attachments.
const (
	Format  = "spokebook-backup"
	Version = 2
	// DataFile is the JSON file in the archive.
	DataFile = "spokebook-backup.json"
)

// The types below are the file format. They are independent of the store
// types, so that the format only changes on purpose; add fields with
// omitempty and raise Version for incompatible changes.

// File is a backup.
type File struct {
	Format         string          `json:"format"`
	Version        int             `json:"version"`
	ExportedAt     time.Time       `json:"exported_at"`
	ComponentTypes []ComponentType `json:"component_types"`
	Bikes          []Bike          `json:"bikes"`
	Components     []Component     `json:"components"`
	Steps          []Step          `json:"steps"`
	// LevelIntervals are the intervals the owner set; missing levels have
	// their default.
	LevelIntervals []LevelInterval `json:"level_intervals,omitempty"`
	Rides          []Ride          `json:"rides,omitempty"`
	RideGears      []RideGear      `json:"ride_gears,omitempty"`
	Attachments    []Attachment    `json:"attachments,omitempty"`
	ShoppingItems  []ShoppingItem  `json:"shopping_items,omitempty"`
}

// Attachment is a photo or document; its content is the file File in the
// archive.
type Attachment struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	ContentType string           `json:"content_type"`
	File        string           `json:"file"`
	CreatedAt   time.Time        `json:"created_at"`
	Links       []AttachmentLink `json:"links"`
}

type AttachmentLink struct {
	BikeID      string `json:"bike_id,omitempty"`
	ComponentID string `json:"component_id,omitempty"`
	Cover       bool   `json:"cover,omitempty"`
}

type Ride struct {
	ID         string    `json:"id"`
	BikeID     string    `json:"bike_id,omitempty"`
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	GearKey    string    `json:"gear_key,omitempty"`
	BikeByGear bool      `json:"bike_by_gear,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	RiddenOn   time.Time `json:"ridden_on"`
	DistanceM  int64     `json:"distance_m"`
	Name       string    `json:"name,omitempty"`
	Discarded  bool      `json:"discarded,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type RideGear struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	GearKey   string    `json:"gear_key"`
	Name      string    `json:"name,omitempty"`
	BikeID    string    `json:"bike_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ShoppingItem struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	BoughtAt  *time.Time `json:"bought_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type LevelInterval struct {
	Level     string `json:"level"`
	Months    *int64 `json:"months,omitempty"`
	DistanceM *int64 `json:"distance_m,omitempty"`
}

type ComponentType struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Hidden    bool      `json:"hidden,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Values    []Value   `json:"values,omitempty"`
}

type Value struct {
	ID    string   `json:"id"`
	Key   string   `json:"key"`
	Kind  string   `json:"kind"`
	Label string   `json:"label"`
	From  *float64 `json:"from,omitempty"`
	To    *float64 `json:"to,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Hint  string   `json:"hint,omitempty"`
}

type Bike struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	Origin       string            `json:"origin,omitempty"`
	PurchasedAt  *time.Time        `json:"purchased_at,omitempty"`
	Dealer       string            `json:"dealer,omitempty"`
	PriceCents   *int64            `json:"price_cents,omitempty"`
	Currency     string            `json:"currency,omitempty"`
	InServiceAt  *time.Time        `json:"in_service_at,omitempty"`
	InServiceM   *int64            `json:"in_service_m,omitempty"`
	Features     []string          `json:"features,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	RetiredAt    *time.Time        `json:"retired_at,omitempty"`
	RetireReason string            `json:"retire_reason,omitempty"`
	Values       []Value           `json:"values,omitempty"`
	Odometer     []OdometerReading `json:"odometer,omitempty"`
	Maintenances []Maintenance     `json:"maintenances,omitempty"`
}

type OdometerReading struct {
	ID         string    `json:"id"`
	RecordedAt time.Time `json:"recorded_at"`
	DistanceM  int64     `json:"distance_m"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

type Maintenance struct {
	ID          string            `json:"id"`
	Title       string            `json:"title,omitempty"`
	Level       string            `json:"level,omitempty"`
	PerformedAt time.Time         `json:"performed_at"`
	DistanceM   int64             `json:"distance_m"`
	Notes       string            `json:"notes,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Steps       []MaintenanceStep `json:"steps,omitempty"`
}

type MaintenanceStep struct {
	ID            string             `json:"id"`
	Position      int                `json:"position"`
	CatalogStepID string             `json:"catalog_step_id,omitempty"`
	BuiltinID     string             `json:"builtin_id,omitempty"`
	Title         string             `json:"title,omitempty"`
	Instructions  string             `json:"instructions,omitempty"`
	Phase         string             `json:"phase"`
	Assembly      string             `json:"assembly,omitempty"`
	Action        string             `json:"action,omitempty"`
	Module        string             `json:"module,omitempty"`
	Hints         []string           `json:"hints,omitempty"`
	OriginID      string             `json:"origin_id,omitempty"`
	Added         bool               `json:"added,omitempty"`
	Done          bool               `json:"done,omitempty"`
	Dismissed     bool               `json:"dismissed,omitempty"`
	Note          string             `json:"note,omitempty"`
	Values        []MaintenanceValue `json:"values,omitempty"`
}

type MaintenanceValue struct {
	Value
	ComponentID       string   `json:"component_id,omitempty"`
	ComponentName     string   `json:"component_name,omitempty"`
	ComponentType     string   `json:"component_type,omitempty"`
	ComponentPosition string   `json:"component_position,omitempty"`
	Measured          *float64 `json:"measured,omitempty"`
}

type Component struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Manufacturer  string         `json:"manufacturer,omitempty"`
	Model         string         `json:"model,omitempty"`
	PartNumber    string         `json:"part_number,omitempty"`
	PurchasedAt   *time.Time     `json:"purchased_at,omitempty"`
	Dealer        string         `json:"dealer,omitempty"`
	PriceCents    *int64         `json:"price_cents,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	MaxDistanceM  *int64         `json:"max_distance_m,omitempty"`
	ReplaceBy     *time.Time     `json:"replace_by,omitempty"`
	URL           string         `json:"url,omitempty"`
	Notes         string         `json:"notes,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	RetiredAt     *time.Time     `json:"retired_at,omitempty"`
	RetireReason  string         `json:"retire_reason,omitempty"`
	Values        []Value        `json:"values,omitempty"`
	Installations []Installation `json:"installations,omitempty"`
}

type Installation struct {
	ID          string    `json:"id"`
	BikeID      string    `json:"bike_id"`
	Position    string    `json:"position,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
	InstalledM  int64     `json:"installed_m"`
	// InstalledAuto and RemovedAuto mean the distance follows the bike's
	// computed odometer.
	InstalledAuto bool       `json:"installed_auto,omitempty"`
	RemovedAt     *time.Time `json:"removed_at,omitempty"`
	RemovedM      *int64     `json:"removed_m,omitempty"`
	RemovedAuto   bool       `json:"removed_auto,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Step struct {
	ID             string     `json:"id"`
	BuiltinID      string     `json:"builtin_id,omitempty"`
	Hidden         bool       `json:"hidden,omitempty"`
	Position       int        `json:"position"`
	Title          string     `json:"title,omitempty"`
	Instructions   string     `json:"instructions,omitempty"`
	Phase          string     `json:"phase"`
	Assembly       string     `json:"assembly,omitempty"`
	Action         string     `json:"action,omitempty"`
	Module         string     `json:"module,omitempty"`
	Replaces       string     `json:"replaces,omitempty"`
	Levels         []string   `json:"levels,omitempty"`
	ComponentTypes []string   `json:"component_types,omitempty"`
	ValueKinds     []string   `json:"value_kinds,omitempty"`
	Values         []Value    `json:"values,omitempty"`
	IntervalMonths *int64     `json:"interval_months,omitempty"`
	IntervalM      *int64     `json:"interval_m,omitempty"`
	FollowUps      []FollowUp `json:"follow_ups,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// FollowUp is a step a step leads to, with the kind ("finding" or
// "always").
type FollowUp struct {
	StepID string `json:"step_id"`
	Kind   string `json:"kind"`
}
