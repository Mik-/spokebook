package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// shoppingOrigin is the page an item is added from, which the form returns
// to: an open maintenance or a component. The zero value is the list.
type shoppingOrigin struct {
	// Kind is "maintenance" or "component".
	Kind string
	ID   string
	Name string
	// Bike is the maintenance's bike, for the breadcrumbs.
	Bike store.Bike
	// Prefill is the text suggested for a new item.
	Prefill string
}

// Query is the query string that keeps the origin on the form page.
func (o shoppingOrigin) Query() string {
	if o.Kind == "" {
		return ""
	}
	return "?" + o.Kind + "=" + o.ID
}

// URL is the page to return to after adding an item.
func (o shoppingOrigin) URL() string {
	switch o.Kind {
	case "maintenance":
		return "/maintenances/" + o.ID
	case "component":
		return "/components/" + o.ID
	}
	return "/shopping"
}

// loadShoppingOrigin reads the origin from the "maintenance" or "component"
// parameter; an ID of another owner is store.ErrNotFound.
func (s *Server) loadShoppingOrigin(ctx context.Context, r *http.Request) (shoppingOrigin, error) {
	l := i18n.FromContext(ctx)
	if id := r.FormValue("maintenance"); id != "" {
		m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), id)
		if err != nil {
			return shoppingOrigin{}, err
		}
		b, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), m.BikeID)
		if err != nil {
			return shoppingOrigin{}, err
		}
		return shoppingOrigin{Kind: "maintenance", ID: m.ID, Name: maintenanceName(l, m), Bike: b}, nil
	}
	if id := r.FormValue("component"); id != "" {
		v, err := s.oneComponentView(ctx, id)
		if err != nil {
			return shoppingOrigin{}, err
		}
		name := strings.TrimSpace(displayName(v))
		prefill := name
		if v.PartNumber != "" {
			prefill += " (" + v.PartNumber + ")"
		}
		return shoppingOrigin{Kind: "component", ID: v.ID, Name: name, Prefill: prefill}, nil
	}
	return shoppingOrigin{}, nil
}

func (s *Server) listShopping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.store.Shopping().ListShoppingItems(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bought := 0
	for _, i := range items {
		if i.Bought() {
			bought++
		}
	}
	s.render(w, r, http.StatusOK, "shopping.html", map[string]any{"Items": items, "Bought": bought})
}

func (s *Server) newShoppingItem(w http.ResponseWriter, r *http.Request) {
	o, err := s.loadShoppingOrigin(r.Context(), r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	form := r.URL.Query()
	form.Set("text", o.Prefill)
	s.render(w, r, http.StatusOK, "shopping_new.html", map[string]any{"Origin": o, "Form": form})
}

func (s *Server) createShoppingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	o, err := s.loadShoppingOrigin(ctx, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		s.render(w, r, http.StatusUnprocessableEntity, "shopping_new.html", map[string]any{
			"Origin": o, "Error": i18n.FromContext(ctx).T("error.text_required"),
		})
		return
	}
	err = s.store.Shopping().CreateShoppingItem(ctx, store.ShoppingItem{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(ctx), Text: text, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, o.URL(), http.StatusSeeOther)
}

func (s *Server) editShoppingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	item, err := s.store.Shopping().GetShoppingItem(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	form := url.Values{"text": {item.Text}}
	s.render(w, r, http.StatusOK, "shopping_edit.html", map[string]any{"Item": item, "Form": form})
}

func (s *Server) updateShoppingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	item, err := s.store.Shopping().GetShoppingItem(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		s.render(w, r, http.StatusUnprocessableEntity, "shopping_edit.html", map[string]any{
			"Item": item, "Error": i18n.FromContext(ctx).T("error.text_required"),
		})
		return
	}
	if err := s.store.Shopping().UpdateShoppingItem(ctx, ownerID(ctx), item.ID, text); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/shopping", http.StatusSeeOther)
}

// setShoppingItemBought ticks an item off (bought) or opens it again.
func (s *Server) setShoppingItemBought(bought bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var at *time.Time
		if bought {
			now := time.Now().UTC()
			at = &now
		}
		if err := s.store.Shopping().SetShoppingItemBought(r.Context(), ownerID(r.Context()), r.PathValue("id"), at); err != nil {
			s.fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/shopping", http.StatusSeeOther)
	}
}

func (s *Server) confirmDeleteShoppingItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Shopping().GetShoppingItem(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, i18n.FromContext(r.Context()).T("confirm.delete_shopping_item", item.Text),
		"/shopping/"+item.ID+"/delete", "/shopping/"+item.ID+"/edit")
}

func (s *Server) deleteShoppingItem(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Shopping().DeleteShoppingItem(r.Context(), ownerID(r.Context()), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/shopping", http.StatusSeeOther)
}

func (s *Server) confirmDeleteBought(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	s.render(w, r, http.StatusOK, "confirm.html", map[string]any{
		"Message": l.T("confirm.remove_bought"), "Action": "/shopping/bought/delete", "Cancel": "/shopping",
		"Submit": l.T("shopping.remove_bought"),
	})
}

func (s *Server) deleteBought(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.Shopping().DeleteBoughtShoppingItems(r.Context(), ownerID(r.Context())); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/shopping", http.StatusSeeOther)
}
