// Package store defines the data layer interfaces. Business logic only knows
// these interfaces, never a concrete database.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

// Bike is a bicycle. All times are UTC.
type Bike struct {
	ID      string
	OwnerID string
	Name    string
	Type    string
	Notes   string
	// Origin is how the bike was acquired, e.g. "self_built"; empty if unknown.
	Origin      string
	PurchasedAt *time.Time
	Dealer      string
	PriceCents  *int64
	Currency    string
	// InServiceAt and InServiceM: since when the bike is used and its
	// odometer then (0 for new bikes, the taken-over value for used ones).
	InServiceAt *time.Time
	InServiceM  *int64
	// Features such as "carbon" switch on modules of the step catalog.
	Features  []string
	CreatedAt time.Time
	// RetiredAt is set when the bike is no longer in use (e.g. sold).
	RetiredAt    *time.Time
	RetireReason string
}

// Active reports whether the bike is not retired.
func (b Bike) Active() bool { return b.RetiredAt == nil }

type BikeStore interface {
	CreateBike(ctx context.Context, b Bike) (Bike, error)
	GetBike(ctx context.Context, ownerID, id string) (Bike, error)
	ListBikes(ctx context.Context, ownerID string) ([]Bike, error)
	// UpdateBike changes all fields except owner, creation and retirement.
	UpdateBike(ctx context.Context, b Bike) error
	// DeleteBike also deletes the bike's odometer readings, installations
	// and values, unassigns its rides and gear mappings and deletes
	// attachments that belonged only to it.
	DeleteBike(ctx context.Context, ownerID, id string) error
	// SetBikeRetired retires the bike, or reactivates it if at is nil.
	SetBikeRetired(ctx context.Context, ownerID, id string, at *time.Time, reason string) error
	// ListBikeValues returns the bike's measurements (saddle height, stack
	// ...) in their order.
	ListBikeValues(ctx context.Context, ownerID, bikeID string) ([]Value, error)
	// ReplaceBikeValues replaces all values of the bike by vs.
	ReplaceBikeValues(ctx context.Context, ownerID, bikeID string, vs []Value) error
	// ListBikeValueLabels and ListBikeValueUnits return the labels and
	// units used in values of the owner's bikes, sorted.
	ListBikeValueLabels(ctx context.Context, ownerID string) ([]string, error)
	ListBikeValueUnits(ctx context.Context, ownerID string) ([]string, error)
}

// Odometer sources.
const (
	SourceManual = "manual"
	SourceImport = "import"
)

// OdometerReading is a bike's total distance at a point in time.
type OdometerReading struct {
	ID         string
	OwnerID    string
	BikeID     string
	RecordedAt time.Time
	DistanceM  int64
	Source     string
	CreatedAt  time.Time
}

type OdometerStore interface {
	// AddReading returns ErrNotFound if the bike does not belong to the owner.
	AddReading(ctx context.Context, r OdometerReading) (OdometerReading, error)
	// ListReadings returns the readings of a bike, newest first.
	ListReadings(ctx context.Context, ownerID, bikeID string) ([]OdometerReading, error)
	GetReading(ctx context.Context, ownerID, id string) (OdometerReading, error)
	// UpdateReading changes date and distance.
	UpdateReading(ctx context.Context, r OdometerReading) error
	DeleteReading(ctx context.Context, ownerID, id string) error
	// PrevReading returns the highest reading at or before at, NextReading
	// the lowest reading after at, both ignoring the reading excludeID.
	// They return ErrNotFound if there is none.
	PrevReading(ctx context.Context, ownerID, bikeID string, at time.Time, excludeID string) (OdometerReading, error)
	NextReading(ctx context.Context, ownerID, bikeID string, at time.Time, excludeID string) (OdometerReading, error)
	// ListAllReadings returns the readings of all bikes, oldest first.
	ListAllReadings(ctx context.Context, ownerID string) ([]OdometerReading, error)
}

// Ride is a ride imported from a file or another source. Only its
// distance counts, on the day RiddenOn (the local date of the start).
type Ride struct {
	ID      string
	OwnerID string
	// BikeID is empty while the ride is not assigned.
	BikeID     string
	Source     string
	ExternalID string
	// GearKey is the bike's key in the source; empty if unknown.
	GearKey string
	// BikeByGear means BikeID came from the RideGear of GearKey.
	BikeByGear bool
	StartedAt  time.Time
	EndedAt    time.Time
	RiddenOn   time.Time
	DistanceM  int64
	Name       string
	// Discarded rides don't count but keep a new import from adding them
	// again.
	Discarded bool
	CreatedAt time.Time
}

// RideGear maps a bike's key in a source to a bike (empty BikeID: none).
type RideGear struct {
	ID      string
	OwnerID string
	Source  string
	GearKey string
	// Name is the bike's name in the source.
	Name      string
	BikeID    string
	CreatedAt time.Time
}

// RideSource is a source an owner connected to fetch rides from, e.g.
// Garmin Connect.
type RideSource struct {
	OwnerID string
	Source  string
	// Credentials are sealed (package secret); the store never sees them
	// in plain text.
	Credentials []byte
	// Since is the first day whose rides are fetched.
	Since time.Time
	RideSourceRun
	CreatedAt time.Time
}

// RideSourceRun is the state a run of a source leaves.
type RideSourceRun struct {
	// Cursor is the start of the newest activity processed; nil before the
	// first.
	Cursor    *time.Time
	LastRunAt *time.Time
	// LastError says why the last run stopped; empty if it didn't.
	LastError      string
	LastImported   int
	LastUnassigned int
	// More means the last run hit its limit and left activities to fetch.
	More bool
}

// DayDistance is the distance a bike was ridden on a day.
type DayDistance struct {
	BikeID    string
	Day       time.Time
	DistanceM int64
}

type RideStore interface {
	// CreateRide returns ErrConflict if the source already has a ride with
	// this external ID and ErrNotFound if the bike does not belong to the
	// owner.
	CreateRide(ctx context.Context, r Ride) (Ride, error)
	GetRide(ctx context.Context, ownerID, id string) (Ride, error)
	FindRide(ctx context.Context, ownerID, source, externalID string) (Ride, error)
	// ListRides returns all rides, newest first.
	ListRides(ctx context.Context, ownerID string) ([]Ride, error)
	// OverlappingRides returns the rides that overlap the time from since
	// to until.
	OverlappingRides(ctx context.Context, ownerID string, since, until time.Time) ([]Ride, error)
	SetRideBike(ctx context.Context, ownerID, id, bikeID string, byGear bool) error
	SetRideDiscarded(ctx context.Context, ownerID, id string, discarded bool) error
	DeleteRide(ctx context.Context, ownerID, id string) error
	// DailyDistances returns the distance per bike and day of the assigned
	// rides that are not discarded, by bike and day.
	DailyDistances(ctx context.Context, ownerID string) ([]DayDistance, error)
	// CreateRideGear returns ErrConflict if the source's key is mapped.
	CreateRideGear(ctx context.Context, g RideGear) (RideGear, error)
	GetRideGear(ctx context.Context, ownerID, id string) (RideGear, error)
	FindRideGear(ctx context.Context, ownerID, source, gearKey string) (RideGear, error)
	ListRideGears(ctx context.Context, ownerID string) ([]RideGear, error)
	SetRideGearBike(ctx context.Context, ownerID, id, bikeID string) error
	// DeleteRideGear deletes a mapping; its rides keep their bike.
	DeleteRideGear(ctx context.Context, ownerID, id string) error

	// CreateRideSource returns ErrConflict if the source is connected.
	CreateRideSource(ctx context.Context, src RideSource) error
	GetRideSource(ctx context.Context, ownerID, source string) (RideSource, error)
	// ListAllRideSources returns the connected sources of all owners.
	ListAllRideSources(ctx context.Context) ([]RideSource, error)
	SetRideSourceCredentials(ctx context.Context, ownerID, source string, credentials []byte) error
	// SetRideSourceSince changes the first day and resets the cursor
	// (nil: start again at since).
	SetRideSourceSince(ctx context.Context, ownerID, source string, since time.Time, cursor *time.Time) error
	SetRideSourceRun(ctx context.Context, ownerID, source string, run RideSourceRun) error
	// DeleteRideSource disconnects the source and forgets its processed
	// items; its rides stay.
	DeleteRideSource(ctx context.Context, ownerID, source string) error
	HasRideSourceItem(ctx context.Context, ownerID, source, itemID string) (bool, error)
	// AddRideSourceItem marks an item as processed; adding it again does
	// nothing.
	AddRideSourceItem(ctx context.Context, ownerID, source, itemID string, at time.Time) error
}

// Component is a part that can be installed on bikes. Optional fields are
// nil or empty. Money is in cents of Currency (ISO 4217).
type Component struct {
	ID           string
	OwnerID      string
	Type         string
	Manufacturer string
	Model        string
	// PartNumber is the manufacturer's part number.
	PartNumber   string
	PurchasedAt  *time.Time
	Dealer       string
	PriceCents   *int64
	Currency     string
	MaxDistanceM *int64
	ReplaceBy    *time.Time
	// URL is a link, e.g. to a data sheet.
	URL       string
	Notes     string
	CreatedAt time.Time
	// RetiredAt is set when the component is no longer in use.
	RetiredAt    *time.Time
	RetireReason string
}

// Active reports whether the component is not retired.
func (c Component) Active() bool { return c.RetiredAt == nil }

// ComponentType is a kind of component. Built-in types have a fixed ID such
// as "chain" and an empty Name until renamed; own types have a UUID.
type ComponentType struct {
	OwnerID   string
	ID        string
	Name      string
	Hidden    bool
	CreatedAt time.Time
}

// Value is a named number such as a torque or a wear limit. From alone is
// a minimum, To alone a maximum, both a range; From == To is a fixed value.
// At least one of them is set and To is not below From. Key is stable
// (derived from the label) and lets a component's value override the
// default of its type. Kind tells what the value is for, Hint adds e.g.
// the paste or threadlocker to use.
type Value struct {
	ID       string
	OwnerID  string
	Position int
	Key      string
	Kind     string
	Label    string
	From     *float64
	To       *float64
	Unit     string
	Hint     string
}

// Value kinds, see Value.Kind. A torque is only shown; a limit (with
// exactly one bound) and a target are measured. An info (e.g. stack or
// tire width) is only shown and never copied into a maintenance.
const (
	KindTorque = "torque"
	KindLimit  = "limit"
	KindTarget = "target"
	KindInfo   = "info"
)

// ValueKinds are all value kinds in display order.
var ValueKinds = []string{KindTorque, KindLimit, KindTarget, KindInfo}

// StepValueKinds are the kinds of component values a step can show.
var StepValueKinds = []string{KindTorque, KindLimit, KindTarget}

// Value bounds, see Value.Bounds.
const (
	BoundsExact = "exact"
	BoundsMin   = "min"
	BoundsMax   = "max"
	BoundsRange = "range"
)

// Bounds tells how From and To are meant.
func (v Value) Bounds() string {
	switch {
	case v.To == nil:
		return BoundsMin
	case v.From == nil:
		return BoundsMax
	case *v.From == *v.To:
		return BoundsExact
	default:
		return BoundsRange
	}
}

// Measurable reports whether a measurement is taken and checked against
// the value, i.e. it is a limit or a target.
func (v Value) Measurable() bool { return v.Kind == KindLimit || v.Kind == KindTarget }

// Allows reports whether x satisfies the value as a limit or range.
func (v Value) Allows(x float64) bool {
	return (v.From == nil || x >= *v.From) && (v.To == nil || x <= *v.To)
}

// Installation is a period in which a component is mounted on a bike.
// InstalledM and RemovedM are the bike's odometer at that time. RemovedAt
// and RemovedM are nil while the component is still installed.
// InstalledAuto and RemovedAuto mean the distance was not entered but
// follows the bike's computed odometer at that date.
type Installation struct {
	ID          string
	OwnerID     string
	ComponentID string
	BikeID      string
	// Position on the bike, e.g. "front"; empty if not applicable.
	Position      string
	InstalledAt   time.Time
	InstalledM    int64
	InstalledAuto bool
	RemovedAt     *time.Time
	RemovedM      *int64
	RemovedAuto   bool
	CreatedAt     time.Time
}

// Open reports whether the component is still installed.
func (i Installation) Open() bool { return i.RemovedAt == nil }

type ComponentStore interface {
	// SeedType adds a built-in type unless it exists (even if hidden).
	SeedType(ctx context.Context, ownerID, id string, at time.Time) error
	CreateType(ctx context.Context, t ComponentType) (ComponentType, error)
	// ListTypes returns all types including hidden ones.
	ListTypes(ctx context.Context, ownerID string) ([]ComponentType, error)
	// RenameType and HideType return ErrNotFound for unknown or hidden types.
	RenameType(ctx context.Context, ownerID, id, name string) error
	HideType(ctx context.Context, ownerID, id string) error
	// ShowType returns ErrNotFound for unknown or visible types.
	ShowType(ctx context.Context, ownerID, id string) error
	CountComponentsOfType(ctx context.Context, ownerID, typeID string) (int64, error)

	CreateComponent(ctx context.Context, c Component) (Component, error)
	GetComponent(ctx context.Context, ownerID, id string) (Component, error)
	ListComponents(ctx context.Context, ownerID string) ([]Component, error)
	// UpdateComponent changes all fields except owner and creation time.
	UpdateComponent(ctx context.Context, c Component) error
	// DeleteComponent also deletes its installations and attachments that
	// belonged only to it.
	DeleteComponent(ctx context.Context, ownerID, id string) error
	// SetComponentRetired retires the component, or reactivates it if at is nil.
	SetComponentRetired(ctx context.Context, ownerID, id string, at *time.Time, reason string) error
	// ListDealers returns the distinct dealers used so far, sorted.
	ListDealers(ctx context.Context, ownerID string) ([]string, error)
	// ListManufacturers returns the distinct manufacturers used so far, sorted.
	ListManufacturers(ctx context.Context, ownerID string) ([]string, error)

	// ListComponentValues returns a component's values in order.
	ListComponentValues(ctx context.Context, ownerID, componentID string) ([]Value, error)
	// ReplaceComponentValues deletes a component's values and adds vs in
	// their order. Call it within WithTx.
	ReplaceComponentValues(ctx context.Context, ownerID, componentID string, vs []Value) error
	// ListTypeValues returns the default values of a component type in order.
	ListTypeValues(ctx context.Context, ownerID, typeID string) ([]Value, error)
	// ReplaceTypeValues deletes a type's default values and adds vs in
	// their order. Call it within WithTx.
	ReplaceTypeValues(ctx context.Context, ownerID, typeID string, vs []Value) error
	// ListValueLabels and ListValueUnits return the distinct labels and
	// units of all values used so far, sorted.
	ListValueLabels(ctx context.Context, ownerID string) ([]string, error)
	ListValueUnits(ctx context.Context, ownerID string) ([]string, error)

	// AddInstallation returns ErrConflict if the component is already
	// installed and ErrNotFound if component or bike don't belong to the owner.
	AddInstallation(ctx context.Context, i Installation) (Installation, error)
	GetInstallation(ctx context.Context, ownerID, id string) (Installation, error)
	// CloseInstallation returns ErrNotFound if there is no open installation
	// with this ID.
	CloseInstallation(ctx context.Context, ownerID, id string, removedAt time.Time, removedM int64, auto bool) error
	// ListInstallations returns a component's installations, newest first.
	ListInstallations(ctx context.Context, ownerID, componentID string) ([]Installation, error)
	// UpdateInstallation changes bike, position, installation and removal.
	UpdateInstallation(ctx context.Context, i Installation) error
	// SetInstallationDistances changes only the distances.
	SetInstallationDistances(ctx context.Context, ownerID, id string, installedM int64, removedM *int64) error
	DeleteInstallation(ctx context.Context, ownerID, id string) error
	CountInstallationsOnBike(ctx context.Context, ownerID, bikeID string) (int64, error)
	// ListAllInstallations returns all installations of the owner, oldest first.
	ListAllInstallations(ctx context.Context, ownerID string) ([]Installation, error)
}

// Step is a step of the owner's catalog. Steps are ordered by Phase,
// Assembly (the assembly group, empty for none) and Position; Action is
// optional. Levels name the maintenance levels the step belongs to, none
// means on demand. ComponentTypes name the component types whose values of
// the ValueKinds apply to the step; Values are its own values. A built-in
// step has a BuiltinID such as "chain_wear"; while its Title and
// Instructions are empty they are translated. Built-in steps are Hidden
// instead of deleted. A step with a Module (a bike feature) applies only to
// bikes with that feature; on those it Replaces the step with that ID. Its
// own Interval makes it due regardless of its levels. FollowUps are the
// steps it can lead to, in order.
type Step struct {
	ID             string
	OwnerID        string
	BuiltinID      string
	Hidden         bool
	Position       int
	Title          string
	Instructions   string
	Phase          string
	Assembly       string
	Action         string
	Module         string
	Replaces       string
	Levels         []string
	ComponentTypes []string
	ValueKinds     []string
	Values         []Value
	Interval       Interval
	FollowUps      []FollowUp
	CreatedAt      time.Time
}

// Kinds of follow-ups, see FollowUp.
const (
	// FollowUpFinding is offered during a maintenance, e.g. worn pads lead
	// to replacing them.
	FollowUpFinding = "finding"
	// FollowUpAlways comes along whenever the step is part of a
	// maintenance, e.g. a new chain needs the derailleur adjusted.
	FollowUpAlways = "always"
)

// FollowUpKinds are all kinds of follow-ups in display order.
var FollowUpKinds = []string{FollowUpFinding, FollowUpAlways}

// FollowUp is a catalog step another step leads to.
type FollowUp struct {
	StepID string
	Kind   string
}

// Interval says after how many months or meters something is due again,
// whatever comes first; nil means no limit of that kind.
type Interval struct {
	Months    *int64
	DistanceM *int64
}

// IsZero reports whether the interval has no limit at all.
func (i Interval) IsZero() bool { return i.Months == nil && i.DistanceM == nil }

// LevelInterval is the interval an owner set for a maintenance level.
type LevelInterval struct {
	Level string
	Interval
}

type StepStore interface {
	// CreateStep and UpdateStep write the step with its levels, component
	// types, value kinds and values (replacing the old ones), so call them
	// within WithTx.
	CreateStep(ctx context.Context, s Step) error
	UpdateStep(ctx context.Context, s Step) error
	GetStep(ctx context.Context, ownerID, id string) (Step, error)
	// ListSteps returns the catalog including hidden steps, ordered by
	// position only.
	ListSteps(ctx context.Context, ownerID string) ([]Step, error)
	SetStepPosition(ctx context.Context, ownerID, id string, position int) error
	HideStep(ctx context.Context, ownerID, id string) error
	ShowStep(ctx context.Context, ownerID, id string) error
	DeleteStep(ctx context.Context, ownerID, id string) error

	// ListLevelIntervals returns the level intervals the owner set.
	ListLevelIntervals(ctx context.Context, ownerID string) ([]LevelInterval, error)
	// SetLevelInterval sets or replaces the interval of a level.
	SetLevelInterval(ctx context.Context, ownerID string, li LevelInterval) error
}

// Maintenance is a performed maintenance of a bike at a Level (empty if it
// has on-demand steps only). Title is the plan name of maintenances from
// before the catalog. Steps and values are copies taken from the catalog.
// It is open until CompletedAt is set.
type Maintenance struct {
	ID          string
	OwnerID     string
	BikeID      string
	Title       string
	Level       string
	PerformedAt time.Time
	DistanceM   int64
	Notes       string
	CompletedAt *time.Time
	CreatedAt   time.Time
	Steps       []MaintenanceStep
}

// Open reports whether the maintenance is not completed yet.
func (m Maintenance) Open() bool { return m.CompletedAt == nil }

// MaintenanceStep is a copy of a catalog step; Position is the step's
// position in the catalog at the time of the copy. Hints are the IDs of
// the module hints that applied to it. A step added as follow-up has the
// ID of the step of the same maintenance that led to it as OriginID; left
// undone it stays open work until a later maintenance does it or it is
// Dismissed. Added marks a step added after the start (not as follow-up).
type MaintenanceStep struct {
	ID            string
	Position      int
	CatalogStepID string
	BuiltinID     string
	Title         string
	Instructions  string
	Phase         string
	Assembly      string
	Action        string
	Module        string
	Hints         []string
	OriginID      string
	Added         bool
	Done          bool
	Dismissed     bool
	Note          string
	Values        []MaintenanceValue
}

// MaintenanceValue is a value of a step. For values of a component, the
// component's ID, name ("Manufacturer Model", may be empty), type and
// installation position are set. Measured is set for measured limits.
type MaintenanceValue struct {
	Value
	ComponentID       string
	ComponentName     string
	ComponentType     string
	ComponentPosition string
	Measured          *float64
}

// OutOfLimit reports whether the measurement violates the value.
func (v MaintenanceValue) OutOfLimit() bool { return v.Measured != nil && !v.Allows(*v.Measured) }

type MaintenanceStore interface {
	// CreateMaintenance writes m with its steps and values, so call it
	// within WithTx. It returns ErrNotFound if the bike doesn't belong to
	// the owner.
	CreateMaintenance(ctx context.Context, m Maintenance) error
	// AddSteps adds steps with their values to a maintenance, so call it
	// within WithTx.
	AddSteps(ctx context.Context, ownerID, maintenanceID string, steps []MaintenanceStep) error
	// GetMaintenance returns the maintenance with its steps and values.
	GetMaintenance(ctx context.Context, ownerID, id string) (Maintenance, error)
	// ListMaintenances returns a bike's maintenances, newest first, without
	// steps.
	ListMaintenances(ctx context.Context, ownerID, bikeID string) ([]Maintenance, error)
	SetMaintenanceNotes(ctx context.Context, ownerID, id, notes string) error
	// SetMaintenanceDate changes date and odometer.
	SetMaintenanceDate(ctx context.Context, ownerID, id string, at time.Time, distanceM int64) error
	// SetMaintenanceCompleted completes the maintenance, or reopens it if at is nil.
	SetMaintenanceCompleted(ctx context.Context, ownerID, id string, at *time.Time) error
	SetStepResult(ctx context.Context, ownerID, stepID string, done bool, note string) error
	SetMeasured(ctx context.Context, ownerID, valueID string, measured *float64) error
	SetStepDismissed(ctx context.Context, ownerID, stepID string, dismissed bool) error
	// DeleteMaintenanceStep deletes a step of a maintenance with its values.
	DeleteMaintenanceStep(ctx context.Context, ownerID, stepID string) error
	DeleteMaintenance(ctx context.Context, ownerID, id string) error
}

// User is an account. Name is the login name, unique regardless of case.
// PasswordHash is opaque to the store. After too many FailedLogins the
// account is locked until LockedUntil.
type User struct {
	ID           string
	Name         string
	PasswordHash string
	FailedLogins int
	LockedUntil  *time.Time
	CreatedAt    time.Time
}

// Session is a login. ID is a hash of the token in the cookie; the session
// ends at ExpiresAt unless it is used before.
type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// Passkey is a WebAuthn credential of a user. Credential holds its data
// (public key, sign count ...) and is opaque to the store.
type Passkey struct {
	ID           string
	UserID       string
	CredentialID []byte
	Name         string
	Credential   []byte
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

type UserStore interface {
	// CreateUser returns ErrConflict if the name is taken.
	CreateUser(ctx context.Context, u User) error
	GetUser(ctx context.Context, id string) (User, error)
	// GetUserByName ignores case.
	GetUserByName(ctx context.Context, name string) (User, error)
	// ListUsers returns all users, oldest first.
	ListUsers(ctx context.Context) ([]User, error)
	SetPassword(ctx context.Context, id, hash string) error
	SetLoginFailures(ctx context.Context, id string, failed int, lockedUntil *time.Time) error

	CreateSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, id string) (Session, error)
	TouchSession(ctx context.Context, id string, lastSeen, expires time.Time) error
	DeleteSession(ctx context.Context, id string) error
	// DeleteUserSessions deletes all sessions of the user except keepID.
	DeleteUserSessions(ctx context.Context, userID, keepID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error

	// CreatePasskey returns ErrConflict if the credential ID is known.
	CreatePasskey(ctx context.Context, p Passkey) error
	GetPasskey(ctx context.Context, userID, id string) (Passkey, error)
	GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (Passkey, error)
	// ListPasskeys returns the user's passkeys, oldest first.
	ListPasskeys(ctx context.Context, userID string) ([]Passkey, error)
	// UpdatePasskeyCredential stores the credential after a login.
	UpdatePasskeyCredential(ctx context.Context, userID, id string, credential []byte, usedAt time.Time) error
	DeletePasskey(ctx context.Context, userID, id string) error

	// DeleteOwnerData deletes all data the user owns (bikes, components,
	// types, steps, level intervals, attachments, the shopping list and
	// everything below them)
	// but keeps the account, sessions and passkeys. Call it within WithTx.
	DeleteOwnerData(ctx context.Context, ownerID string) error
}

// Attachment is a file such as a photo or a receipt that belongs to bikes
// or components (see AttachmentLink). Data and Thumb are only set to create
// one; reading them goes through GetAttachmentData and GetAttachmentThumb.
type Attachment struct {
	ID          string
	OwnerID     string
	Name        string
	ContentType string
	Size        int64
	Data        []byte
	// Thumb is a smaller JPEG of an image; nil for other files.
	Thumb    []byte
	HasThumb bool
	// Cover is set by ListBikeAttachments for the bike's photo.
	Cover     bool
	CreatedAt time.Time
}

// AttachmentLink ties an attachment to either a bike or a component. Cover
// marks the bike's photo; a bike has at most one.
type AttachmentLink struct {
	AttachmentID string
	BikeID       string
	ComponentID  string
	Cover        bool
}

type AttachmentStore interface {
	CreateAttachment(ctx context.Context, a Attachment) error
	// GetAttachment returns the attachment without Data and Thumb.
	GetAttachment(ctx context.Context, ownerID, id string) (Attachment, error)
	// GetAttachmentData returns the content type and the file.
	GetAttachmentData(ctx context.Context, ownerID, id string) (string, []byte, error)
	// GetAttachmentThumb returns ErrNotFound if there is no preview.
	GetAttachmentThumb(ctx context.Context, ownerID, id string) ([]byte, error)
	// The lists leave out Data and Thumb and are newest first; the cover
	// of a bike comes first.
	ListAttachments(ctx context.Context, ownerID string) ([]Attachment, error)
	ListBikeAttachments(ctx context.Context, ownerID, bikeID string) ([]Attachment, error)
	ListComponentAttachments(ctx context.Context, ownerID, componentID string) ([]Attachment, error)
	// ListAttachmentLinks returns the links of one attachment, bikes first.
	ListAttachmentLinks(ctx context.Context, ownerID, id string) ([]AttachmentLink, error)
	// ListAllAttachmentLinks returns the links of all attachments.
	ListAllAttachmentLinks(ctx context.Context, ownerID string) ([]AttachmentLink, error)
	// Covers maps bike IDs to the attachment ID of their photo.
	Covers(ctx context.Context, ownerID string) (map[string]string, error)
	// LinkAttachment returns ErrConflict if the link exists and ErrNotFound
	// if the attachment, bike or component does not belong to the owner.
	// A link with Cover replaces the bike's cover; call it within WithTx.
	LinkAttachment(ctx context.Context, ownerID string, l AttachmentLink) error
	// UnlinkAttachment removes a link and deletes the attachment if it was
	// the last one. Call it within WithTx.
	UnlinkAttachment(ctx context.Context, ownerID string, l AttachmentLink) error
	// SetCover makes the attachment, which must be linked to the bike, its
	// photo; an empty attachmentID removes the photo. Call it within WithTx.
	SetCover(ctx context.Context, ownerID, bikeID, attachmentID string) error
	RenameAttachment(ctx context.Context, ownerID, id, name string) error
	// DeleteAttachment deletes the attachment and its links.
	DeleteAttachment(ctx context.Context, ownerID, id string) error
}

// ShoppingItem is something to buy, e.g. a consumable noticed during a
// maintenance. BoughtAt is set once it is ticked off.
type ShoppingItem struct {
	ID        string
	OwnerID   string
	Text      string
	BoughtAt  *time.Time
	CreatedAt time.Time
}

// Bought reports whether the item is ticked off.
func (i ShoppingItem) Bought() bool { return i.BoughtAt != nil }

type ShoppingStore interface {
	CreateShoppingItem(ctx context.Context, i ShoppingItem) error
	GetShoppingItem(ctx context.Context, ownerID, id string) (ShoppingItem, error)
	// ListShoppingItems returns open items first, then bought ones, each
	// in the order they were added.
	ListShoppingItems(ctx context.Context, ownerID string) ([]ShoppingItem, error)
	UpdateShoppingItem(ctx context.Context, ownerID, id, text string) error
	// SetShoppingItemBought ticks the item off, or opens it again if at is nil.
	SetShoppingItemBought(ctx context.Context, ownerID, id string, at *time.Time) error
	DeleteShoppingItem(ctx context.Context, ownerID, id string) error
	// DeleteBoughtShoppingItems returns how many items were deleted.
	DeleteBoughtShoppingItems(ctx context.Context, ownerID string) (int64, error)
}

// Stores groups all sub-stores, both inside and outside a transaction.
type Stores interface {
	Users() UserStore
	Bikes() BikeStore
	Odometer() OdometerStore
	Rides() RideStore
	Components() ComponentStore
	Steps() StepStore
	Maintenances() MaintenanceStore
	Attachments() AttachmentStore
	Shopping() ShoppingStore
}

// Store is the entry point of the data layer.
type Store interface {
	Stores
	// WithTx runs fn atomically; on error the transaction is rolled back.
	WithTx(ctx context.Context, fn func(Stores) error) error
	Close() error
}
