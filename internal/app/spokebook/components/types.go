package components

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

// ErrNameRequired means an own type needs a name.
var ErrNameRequired = errors.New("components: name required")

// SeedTypes adds the built-in types the owner doesn't have yet. Hidden
// types stay hidden.
func SeedTypes(ctx context.Context, st store.Store, ownerID string) error {
	now := time.Now().UTC()
	return st.WithTx(ctx, func(s store.Stores) error {
		for _, id := range BuiltinTypes {
			if err := s.Components().SeedType(ctx, ownerID, id, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// IsBuiltin reports whether id is a built-in type.
func IsBuiltin(id string) bool { return slices.Contains(BuiltinTypes, id) }

// CreateType adds an own type.
func CreateType(ctx context.Context, st store.Store, ownerID, name string) (store.ComponentType, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return store.ComponentType{}, ErrNameRequired
	}
	return st.Components().CreateType(ctx, store.ComponentType{
		OwnerID: ownerID, ID: uuid.Must(uuid.NewV7()).String(), Name: name, CreatedAt: time.Now().UTC(),
	})
}

// RenameType renames a type. An empty name resets a built-in type to its
// translated name; own types need a name.
func RenameType(ctx context.Context, st store.Store, ownerID, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" && !IsBuiltin(id) {
		return ErrNameRequired
	}
	return st.Components().RenameType(ctx, ownerID, id, name)
}

// DeleteType hides a type that no component uses.
func DeleteType(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		n, err := s.Components().CountComponentsOfType(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrTypeInUse
		}
		return s.Components().HideType(ctx, ownerID, id)
	})
}

// RestoreType shows a deleted (hidden) type again.
func RestoreType(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.Components().ShowType(ctx, ownerID, id)
}

// SetTypeValues replaces the default values of an active type.
func SetTypeValues(ctx context.Context, st store.Store, ownerID, typeID string, vs []store.Value) error {
	vs, err := values.Prepare(ownerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		if _, err := ActiveType(ctx, s, ownerID, typeID); err != nil {
			return err
		}
		return s.Components().ReplaceTypeValues(ctx, ownerID, typeID, vs)
	})
}

// ActiveType returns the type if it exists and is not hidden, else
// ErrInvalidType.
func ActiveType(ctx context.Context, s store.Stores, ownerID, id string) (store.ComponentType, error) {
	types, err := s.Components().ListTypes(ctx, ownerID)
	if err != nil {
		return store.ComponentType{}, err
	}
	for _, t := range types {
		if t.ID == id && !t.Hidden {
			return t, nil
		}
	}
	return store.ComponentType{}, ErrInvalidType
}
