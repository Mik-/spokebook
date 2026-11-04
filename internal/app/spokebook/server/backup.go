package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/backup"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
)

// maxBackupMB limits the size of an uploaded backup.
const maxBackupMB = 2048

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Export into a temporary file so an error doesn't leave a half file
	// and photos don't have to fit into memory.
	tmp, err := os.CreateTemp("", "spokebook-backup-*.zip")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := backup.Write(ctx, s.store, ownerID(ctx), tmp); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		s.fail(w, r, err)
		return
	}
	name := "spokebook-backup-" + time.Now().UTC().Format(time.DateOnly) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, tmp)
}

func (s *Server) importBackupForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "backup_import.html", map[string]any{})
}

func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	invalid := func(msg string) {
		s.render(w, r, http.StatusUnprocessableEntity, "backup_import.html", map[string]any{"Error": msg})
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupMB<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			invalid(l.T("backup.error.too_large", maxBackupMB))
			return
		}
		invalid(l.T("backup.error.file"))
		return
	}
	if r.FormValue("confirm") == "" {
		invalid(l.T("backup.error.confirm"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		invalid(l.T("backup.error.file"))
		return
	}
	defer func() { _ = file.Close() }()

	f, err := backup.Read(file, header.Size)
	if err == nil {
		err = backup.Import(ctx, s.store, ownerID(ctx), f)
	}
	switch {
	case errors.Is(err, backup.ErrVersion):
		invalid(l.T("backup.error.version"))
	case errors.Is(err, backup.ErrInvalid):
		s.log.Warn("backup import rejected", "err", err)
		invalid(l.T("backup.error.invalid"))
	case err != nil:
		s.fail(w, r, err)
	default:
		s.log.Info("backup imported", "user", ownerID(ctx), "bikes", len(f.Bikes), "components", len(f.Components))
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
