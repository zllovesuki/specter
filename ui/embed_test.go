package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedFrontends(t *testing.T) {
	for _, app := range []struct {
		name   string
		prefix string
		page   http.Handler
		assets http.Handler
	}{
		{"client", "/ui/", ClientPage(), Assets()},
		{"operator", "/_internal/ui/", OperatorPage(), Assets()},
	} {
		t.Run(app.name, func(t *testing.T) {
			page := httptest.NewRecorder()
			app.page.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
			if page.Code != http.StatusOK || !strings.Contains(page.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("page: %d %v", page.Code, page.Header())
			}
			if page.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("HTML must revalidate to avoid stale asset references")
			}
			mounted := http.StripPrefix(strings.TrimSuffix(app.prefix, "/"), app.assets)
			refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
			if len(refs) < 2 {
				t.Fatal("expected a script and stylesheet")
			}
			for _, ref := range refs {
				if !strings.HasPrefix(ref[1], app.prefix) {
					t.Fatalf("asset escaped its UI prefix: %s", ref[1])
				}
				response := httptest.NewRecorder()
				mounted.ServeHTTP(response, httptest.NewRequest(http.MethodGet, ref[1], nil))
				if response.Code != http.StatusOK || response.Body.Len() == 0 {
					t.Fatalf("asset %s: status=%d bytes=%d", ref[1], response.Code, response.Body.Len())
				}
				if strings.HasSuffix(ref[1], ".css") {
					base, err := url.Parse("http://preview" + ref[1])
					if err != nil {
						t.Fatal(err)
					}
					for _, font := range regexp.MustCompile(`url\(([^)]+\.woff2[^)]*)\)`).FindAllStringSubmatch(response.Body.String(), -1) {
						fontURL, err := base.Parse(strings.Trim(font[1], "\"'"))
						if err != nil {
							t.Fatal(err)
						}
						got := httptest.NewRecorder()
						mounted.ServeHTTP(got, httptest.NewRequest(http.MethodGet, fontURL.String(), nil))
						if got.Code != http.StatusOK || !strings.HasPrefix(got.Body.String(), "wOF2") {
							t.Fatalf("CSS font URL %s did not serve WOFF2", fontURL)
						}
					}
				}
			}
			// All generated static assets remain reachable beneath either mount.
			err := fs.WalkDir(output, "dist", func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				name = strings.TrimPrefix(name, "dist/")
				if !strings.HasPrefix(name, "assets/") && !strings.HasPrefix(name, "fonts/") {
					return nil
				}
				response := httptest.NewRecorder()
				mounted.ServeHTTP(response, httptest.NewRequest(http.MethodGet, app.prefix+name, nil))
				if response.Code != http.StatusOK {
					t.Errorf("build asset %s: %d", name, response.Code)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			head := httptest.NewRecorder()
			app.page.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/", nil))
			if head.Code != http.StatusOK || head.Body.Len() != 0 {
				t.Fatal("HEAD should return an empty successful response")
			}
		})
	}
}

func TestUIAssetsExposeOnlyStaticBuildFiles(t *testing.T) {
	handler := Assets()
	for _, name := range []string{"operator/index.html", "client/index.html", ".vite/manifest.json", "assets/../client/index.html", "client/main.ts", "fonts/", "assets/", ""} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+name, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("unexpected access to %q: %d", name, response.Code)
		}
	}
}
