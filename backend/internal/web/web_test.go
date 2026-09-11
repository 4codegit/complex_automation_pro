package web

import (
	"io/fs"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHandlerServesSPA(t *testing.T) {
	h := Handler()

	// Discover one real hashed asset from the embedded bundle so the test
	// survives frontend rebuilds (asset names are content-hashed).
	var asset string
	err := fs.WalkDir(content, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if asset == "" && filepath.Base(p) != "index.html" && filepath.Ext(p) == ".js" {
			asset = "/" + p
		}
		return nil
	})
	if err != nil || asset == "" {
		t.Fatalf("no embedded asset found (run the frontend build): %v", err)
	}

	cases := []struct {
		path string
		want int
	}{
		{"/", 200},
		{"/some/route", 200},
		{asset, 200},
		{"/api/v1/health", 404},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("GET %s -> %d, want %d", c.path, rec.Code, c.want)
		}
	}
}
