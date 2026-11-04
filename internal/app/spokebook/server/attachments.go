package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Mik-/spokebook/internal/app/spokebook/attachments"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// maxAttachmentUploadMB limits one upload of several files.
const maxAttachmentUploadMB = 100

// crumb is one entry of the breadcrumbs.
type crumb struct{ Name, Path string }

// attachTarget is the bike or component an attachment page was opened
// from; its breadcrumbs and redirects lead back there.
type attachTarget struct {
	store.AttachmentLink
	Name string
	// Path is the page of the bike or component.
	Path string
	// Crumbs lead up to and include the target.
	Crumbs []crumb
}

// Query keeps the target on the attachment pages.
func (t attachTarget) Query() string {
	if t.BikeID != "" {
		return "?bike=" + url.QueryEscape(t.BikeID)
	}
	return "?component=" + url.QueryEscape(t.ComponentID)
}

func (s *Server) bikeTarget(ctx context.Context, id string) (attachTarget, error) {
	b, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), id)
	if err != nil {
		return attachTarget{}, err
	}
	path := "/bikes/" + b.ID
	return attachTarget{
		AttachmentLink: store.AttachmentLink{BikeID: b.ID}, Name: b.Name, Path: path,
		Crumbs: []crumb{{i18n.FromContext(ctx).T("bikes.title"), "/"}, {b.Name, path}},
	}, nil
}

func (s *Server) componentTarget(ctx context.Context, id string) (attachTarget, error) {
	v, err := s.oneComponentView(ctx, id)
	if err != nil {
		return attachTarget{}, err
	}
	path := "/components/" + v.ID
	name := displayName(v)
	return attachTarget{
		AttachmentLink: store.AttachmentLink{ComponentID: v.ID}, Name: name, Path: path,
		Crumbs: []crumb{{i18n.FromContext(ctx).T("components.title"), "/components"}, {name, path}},
	}, nil
}

// pathTarget is the target of a page below /bikes/{id} or /components/{id}.
func (s *Server) pathTarget(r *http.Request) (attachTarget, error) {
	if strings.HasPrefix(r.URL.Path, "/bikes/") {
		return s.bikeTarget(r.Context(), r.PathValue("id"))
	}
	return s.componentTarget(r.Context(), r.PathValue("id"))
}

// queryTarget is the target an attachment page names by its "bike" or
// "component" parameter; without one (or one the attachment is not linked
// to) it is the first link.
func (s *Server) queryTarget(r *http.Request, links []store.AttachmentLink) (attachTarget, error) {
	if len(links) == 0 {
		return attachTarget{}, store.ErrNotFound
	}
	want := store.AttachmentLink{BikeID: r.FormValue("bike"), ComponentID: r.FormValue("component")}
	l := links[0]
	for _, c := range links {
		if (want.BikeID != "" && c.BikeID == want.BikeID) || (want.ComponentID != "" && c.ComponentID == want.ComponentID) {
			l = c
			break
		}
	}
	if l.BikeID != "" {
		return s.bikeTarget(r.Context(), l.BikeID)
	}
	return s.componentTarget(r.Context(), l.ComponentID)
}

// serveAttachment sends the file, shown in the browser unless the query
// asks for "download". A PDF gets its own policy: the browser's viewer may
// load it as a plugin, but nothing in it may run scripts, load resources
// or frame the app. An attachment never changes, so the browser may keep
// it.
func (s *Server) serveAttachment(w http.ResponseWriter, r *http.Request, a store.Attachment, contentType string, data []byte) {
	disposition := "inline"
	if r.URL.Query().Has("download") {
		disposition = "attachment"
	}
	h := w.Header()
	if !attachments.IsImage(contentType) {
		h.Set("Content-Security-Policy", "default-src 'none'; object-src 'self'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	}
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Name}))
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", a.CreatedAt, bytes.NewReader(data))
}

func (s *Server) attachmentFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.Attachments().GetAttachment(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	contentType, data, err := s.store.Attachments().GetAttachmentData(ctx, ownerID(ctx), a.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.serveAttachment(w, r, a, contentType, data)
}

func (s *Server) attachmentThumb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.Attachments().GetAttachment(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	thumb, err := s.store.Attachments().GetAttachmentThumb(ctx, ownerID(ctx), a.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.serveAttachment(w, r, a, "image/jpeg", thumb)
}

func (s *Server) newAttachments(w http.ResponseWriter, r *http.Request) {
	s.renderNewAttachments(w, r, http.StatusOK, nil, "")
}

func (s *Server) renderNewAttachments(w http.ResponseWriter, r *http.Request, status int, files []fileError, errMsg string) {
	t, err := s.pathTarget(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, status, "attachment_new.html", map[string]any{
		"Target": t, "MaxMB": attachments.MaxFileSize >> 20, "FileErrors": files, "Error": errMsg,
	})
}

// addAttachments stores the uploaded files, all or none.
func (s *Server) addAttachments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	t, err := s.pathTarget(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentUploadMB<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		msg := l.T("attachments.error.upload")
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			msg = l.T("attachments.error.too_large_upload", maxAttachmentUploadMB)
		}
		s.renderNewAttachments(w, r, http.StatusUnprocessableEntity, nil, msg)
		return
	}
	var files []attachments.File
	var failed []fileError
	for _, fh := range r.MultipartForm.File["files"] {
		if fh.Size > attachments.MaxFileSize {
			failed = append(failed, fileError{Name: fh.Filename, Message: l.T("attachments.error.too_large", attachments.MaxFileSize>>20)})
			continue
		}
		f, err := fh.Open()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		prepared, err := attachments.Prepare(fh.Filename, data)
		switch {
		case errors.Is(err, attachments.ErrType):
			failed = append(failed, fileError{Name: fh.Filename, Message: l.T("attachments.error.type")})
		case errors.Is(err, attachments.ErrImage):
			failed = append(failed, fileError{Name: fh.Filename, Message: l.T("attachments.error.image")})
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			files = append(files, prepared)
		}
	}
	if len(failed) > 0 || len(files) == 0 {
		msg := ""
		if len(failed) == 0 {
			msg = l.T("attachments.error.none")
		}
		s.renderNewAttachments(w, r, http.StatusUnprocessableEntity, failed, msg)
		return
	}
	if _, err := attachments.Add(ctx, s.store, ownerID(ctx), t.AttachmentLink, files); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, t.Path+"#attachments", http.StatusSeeOther)
}

// linkCandidate is an attachment that can be linked to a target, with the
// names of what it belongs to.
type linkCandidate struct {
	store.Attachment
	LinkedTo string
}

func (s *Server) linkAttachmentsForm(w http.ResponseWriter, r *http.Request) {
	s.renderLinkAttachments(w, r, http.StatusOK, "")
}

func (s *Server) renderLinkAttachments(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	ctx := r.Context()
	t, err := s.pathTarget(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	all, err := s.store.Attachments().ListAttachments(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.store.Attachments().ListAllAttachmentLinks(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.linkNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	linkedTo := make(map[string][]string)
	skip := make(map[string]bool)
	for _, l := range links {
		if (t.BikeID != "" && l.BikeID == t.BikeID) || (t.ComponentID != "" && l.ComponentID == t.ComponentID) {
			skip[l.AttachmentID] = true
		}
		linkedTo[l.AttachmentID] = append(linkedTo[l.AttachmentID], names[l.BikeID+l.ComponentID])
	}
	var cands []linkCandidate
	for _, a := range all {
		if !skip[a.ID] {
			cands = append(cands, linkCandidate{Attachment: a, LinkedTo: strings.Join(linkedTo[a.ID], ", ")})
		}
	}
	s.render(w, r, status, "attachment_link.html", map[string]any{"Target": t, "Candidates": cands, "Error": errMsg})
}

// linkNames maps the IDs of all bikes and components to their names.
func (s *Server) linkNames(ctx context.Context) (map[string]string, error) {
	names, err := s.bikeNames(ctx)
	if err != nil {
		return nil, err
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		return nil, err
	}
	for id, v := range views {
		names[id] = displayName(v)
	}
	return names, nil
}

func (s *Server) linkAttachments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.pathTarget(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	ids := r.PostForm["attachment"]
	if len(ids) == 0 {
		s.renderLinkAttachments(w, r, http.StatusUnprocessableEntity, i18n.FromContext(ctx).T("attachments.error.none_chosen"))
		return
	}
	if err := attachments.Link(ctx, s.store, ownerID(ctx), t.AttachmentLink, ids); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, t.Path+"#attachments", http.StatusSeeOther)
}

// setCover makes an attachment of the bike its photo; without one the bike
// has no photo, but the attachment stays.
func (s *Server) setCover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bikeID := r.PathValue("id")
	err := s.store.WithTx(ctx, func(tx store.Stores) error {
		return tx.Attachments().SetCover(ctx, ownerID(ctx), bikeID, r.FormValue("attachment"))
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+bikeID+"#attachments", http.StatusSeeOther)
}

// linkView is a bike or component an attachment belongs to.
type linkView struct {
	store.AttachmentLink
	Name string
	Path string
}

func (s *Server) editAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.Attachments().GetAttachment(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderAttachment(w, r, http.StatusOK, a, url.Values{"name": {a.Name}}, "")
}

func (s *Server) renderAttachment(w http.ResponseWriter, r *http.Request, status int, a store.Attachment, form url.Values, errMsg string) {
	ctx := r.Context()
	links, err := s.store.Attachments().ListAttachmentLinks(ctx, ownerID(ctx), a.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.queryTarget(r, links)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.linkNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views := make([]linkView, 0, len(links))
	for _, l := range links {
		v := linkView{AttachmentLink: l, Name: names[l.BikeID+l.ComponentID], Path: "/components/" + l.ComponentID}
		if l.BikeID != "" {
			v.Path = "/bikes/" + l.BikeID
		}
		views = append(views, v)
	}
	data := map[string]any{"A": a, "Target": t, "Links": views, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "attachment_edit.html", data)
}

func (s *Server) updateAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.Attachments().GetAttachment(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = attachments.Rename(ctx, s.store, ownerID(ctx), a.ID, r.FormValue("name"))
	if errors.Is(err, attachments.ErrName) {
		s.renderAttachment(w, r, http.StatusUnprocessableEntity, a, nil, i18n.FromContext(ctx).T("error.name_required"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.redirectToTarget(w, r, a.ID)
}

// redirectToTarget leads back to the page the attachment was opened from.
func (s *Server) redirectToTarget(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	links, err := s.store.Attachments().ListAttachmentLinks(ctx, ownerID(ctx), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.queryTarget(r, links)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, t.Path+"#attachments", http.StatusSeeOther)
}

// unlinkAttachment removes one of several links; the last one goes only by
// deleting the attachment.
func (s *Server) unlinkAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	l := store.AttachmentLink{AttachmentID: id, BikeID: r.FormValue("unlink_bike"), ComponentID: r.FormValue("unlink_component")}
	if (l.BikeID == "") == (l.ComponentID == "") {
		s.errorPage(w, r, http.StatusBadRequest, i18n.FromContext(ctx).T("error.invalid_choice"))
		return
	}
	err := attachments.Unlink(ctx, s.store, ownerID(ctx), l)
	if errors.Is(err, attachments.ErrLastLink) {
		http.Redirect(w, r, "/attachments/"+id+"/delete?"+r.URL.RawQuery, http.StatusSeeOther)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The page it was opened from may have been the removed link.
	q := r.URL.Query()
	if (l.BikeID != "" && q.Get("bike") == l.BikeID) || (l.ComponentID != "" && q.Get("component") == l.ComponentID) {
		target := "/components/" + l.ComponentID
		if l.BikeID != "" {
			target = "/bikes/" + l.BikeID
		}
		http.Redirect(w, r, target+"#attachments", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/attachments/"+id+"/edit?"+r.URL.RawQuery, http.StatusSeeOther)
}

func (s *Server) confirmDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	a, err := s.store.Attachments().GetAttachment(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.store.Attachments().ListAttachmentLinks(ctx, ownerID(ctx), a.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.queryTarget(r, links)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	msg := l.T("confirm.delete_attachment", a.Name)
	if len(links) > 1 {
		msg = l.T("confirm.delete_attachment_linked", a.Name, len(links))
	}
	s.confirm(w, r, msg, "/attachments/"+a.ID+"/delete"+t.Query(), "/attachments/"+a.ID+"/edit"+t.Query())
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	links, err := s.store.Attachments().ListAttachmentLinks(ctx, ownerID(ctx), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.queryTarget(r, links)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.Attachments().DeleteAttachment(ctx, ownerID(ctx), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, t.Path+"#attachments", http.StatusSeeOther)
}

// coverOf returns the bike's photo from its attachments, or nil.
func coverOf(list []store.Attachment) *store.Attachment {
	i := slices.IndexFunc(list, func(a store.Attachment) bool { return a.Cover })
	if i < 0 {
		return nil
	}
	return &list[i]
}
