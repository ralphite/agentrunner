package tool

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fetchServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/plain", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "plain body here")
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>t</title><style>p{color:red}</style></head>
<body><!-- hidden --><script>alert("evil")</script>
<h1>Header</h1><p>First &amp; second.</p><div>Block</div></body></html>`)
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/plain", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("x", webFetchReadBytes+1000))
	})
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "boom detail")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestWebFetchPlainText(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/plain"))
	if isErr {
		t.Fatalf("errored: %v", m)
	}
	if m["content"] != "plain body here" || m["status"].(float64) != 200 {
		t.Fatalf("unexpected payload: %v", m)
	}
	if m["untrusted_content"] != true {
		t.Fatalf("missing untrusted_content marker: %v", m)
	}
}

func TestWebFetchHTMLToText(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/page"))
	if isErr {
		t.Fatalf("errored: %v", m)
	}
	content := m["content"].(string)
	for _, want := range []string{"Header", "First & second.", "Block"} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q: %q", want, content)
		}
	}
	for _, drop := range []string{"alert", "color:red", "hidden", "<h1>", "title"} {
		if strings.Contains(content, drop) {
			t.Errorf("content leaked %q: %q", drop, content)
		}
	}
}

func TestWebFetchFollowsRedirects(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/redir"))
	if isErr {
		t.Fatalf("errored: %v", m)
	}
	if m["content"] != "plain body here" {
		t.Fatalf("redirect not followed: %v", m)
	}
	if got := m["url"].(string); !strings.HasSuffix(got, "/plain") {
		t.Fatalf("final url = %q, want .../plain", got)
	}
}

func TestWebFetchRedirectLoopStops(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/loop"))
	if !isErr || !strings.Contains(m["error"].(string), "redirects") {
		t.Fatalf("want redirect-cap error, got %v (isErr=%v)", m, isErr)
	}
}

func TestWebFetchRejectsNonHTTP(t *testing.T) {
	e, _ := newExec(t)
	for _, u := range []string{"ftp://example.com/x", "file:///etc/passwd", "notaurl", ""} {
		m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, u))
		if !isErr {
			t.Errorf("url %q accepted: %v", u, m)
		}
	}
}

func TestWebFetchTruncatesOversizedBody(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/big"))
	if isErr {
		t.Fatalf("errored: %v", m)
	}
	if m["truncated"] != true {
		t.Fatalf("want truncated=true, got %v", m)
	}
	if len(m["content"].(string)) > webFetchOutputBytes {
		t.Fatalf("content over output cap: %d bytes", len(m["content"].(string)))
	}
}

func TestWebFetchRejectsBinary(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/bin"))
	if !isErr || !strings.Contains(m["error"].(string), "content type") {
		t.Fatalf("want unsupported-content-type error, got %v (isErr=%v)", m, isErr)
	}
}

func TestWebFetchHTTPErrorIsModelVisible(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/broken"))
	if !isErr {
		t.Fatalf("want IsError on 500, got %v", m)
	}
	// The body still renders (决策 #9: errors are model-visible results) so
	// the model can react to the failure detail.
	if m["status"].(float64) != 500 || !strings.Contains(m["content"].(string), "boom detail") {
		t.Fatalf("want status 500 with body, got %v", m)
	}
}

// M2 (security review): link-local / cloud-metadata addresses are never a
// valid fetch target — the egress guard refuses them before any bytes flow,
// even without containment. This blocks IAM-credential theft on cloud/CI.
func TestWebFetchRefusesLinkLocalMetadata(t *testing.T) {
	e, _ := newExec(t)
	// 169.254.169.254 = AWS/GCP/Azure metadata; 169.254.170.2 = ECS creds.
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://169.254.170.2/v2/credentials",
	} {
		m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, u))
		if !isErr || !strings.Contains(m["error"].(string), "link-local") {
			t.Errorf("url %q not refused: %v (isErr=%v)", u, m, isErr)
		}
	}
}

// The guard predicate itself: link-local literals refused, ordinary refused-set
// is minimal (public IPs pass; the guard only judges IP literals it is handed).
func TestRefuseLinkLocalPredicate(t *testing.T) {
	refused := []string{"169.254.169.254:80", "169.254.170.2:80", "[fe80::1]:80"}
	for _, a := range refused {
		if refuseLinkLocal("tcp", a, nil) == nil {
			t.Errorf("%s should be refused", a)
		}
	}
	allowed := []string{"93.184.216.34:443", "8.8.8.8:53", "[2606:2800:220:1:248:1893:25c8:1946]:443"}
	for _, a := range allowed {
		if err := refuseLinkLocal("tcp", a, nil); err != nil {
			t.Errorf("%s should pass, got %v", a, err)
		}
	}
}

// The containment ratchet reaches web_fetch as FAIL CLOSED: an in-process
// fetch cannot be netns-wrapped like bash, so under network=none it refuses
// to run at all — never silent egress (INC-5 / S7 模块 5 discipline).
func TestWebFetchFailsClosedUnderContainment(t *testing.T) {
	e, _ := newExec(t)
	srv := fetchServer(t)
	e.ContainNetwork()
	m, isErr := run(t, e, "web_fetch", fmt.Sprintf(`{"url":%q}`, srv.URL+"/plain"))
	if !isErr || !strings.Contains(m["error"].(string), "network=none") {
		t.Fatalf("want fail-closed refusal, got %v (isErr=%v)", m, isErr)
	}
}
