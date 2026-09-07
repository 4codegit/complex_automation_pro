package web

import (
	"net/http/httptest"
	"testing"
)

func TestHandlerServesSPA(t *testing.T) {
	h := Handler()

	cases := []struct {
		path string
		want int
	}{
		{"/", 200},
		{"/some/route", 200},
		{"/assets/index-BSpWPenB.js", 200},
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
