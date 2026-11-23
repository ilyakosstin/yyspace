package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

func TestMergeMarkdown(t *testing.T) {
	for _, tt := range []struct{ name, base, current, incoming, want string }{
		{"independent edits", "first\nsecond\nthird\n", "FIRST\nsecond\nthird\n", "first\nsecond\nTHIRD\n", "FIRST\nsecond\nTHIRD\n"},
		{"overlapping edits", "# Notes\nold\n", "# Notes\nfrom one\nshared\n", "# Notes\nfrom two\nshared\n", "# Notes\nfrom one\nfrom two\nshared\n"},
		{"same insertion", "A\nB", "A\nnew\nB", "A\nnew\nB", "A\nnew\nB"},
		{"two insertions", "A\nB", "A\nnew\nB", "A\nother\nB", "A\nnew\nother\nB"},
		{"adjacent insert and replace", "A\nB", "A\nnew\nB", "A\nother", "A\nnew\nother"},
		{"deletion alone", "A\nB\nC", "A\nC", "A\nB\nC!", "A\nC!"},
		{"deletion versus edit", "A\nB\nC", "A\nC", "A\nB!\nC", "A\nB!\nC"},
		{"whole document deletion versus edit", "old", "", "new", "new"},
		{"duplicate lines", "header\nold\nend", "header\na\na\nb\nend", "header\na\nc\nend", "header\na\na\nb\nc\nend"},
		{"repeat stale save", "top\nold\nend", "top\na\nb\nend", "top\nb\nend", "top\na\nb\nend"},
		{"empty starting document", "", "left", "right", "left\nright"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeMarkdown(tt.base, tt.current, tt.incoming); got != tt.want {
				t.Fatalf("merge = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeWideMarkdown(t *testing.T) {
	base := make([]string, 1800)
	for i := range base {
		base[i] = fmt.Sprintf("line %d", i)
	}
	current, incoming, want := append([]string(nil), base...), append([]string(nil), base...), append([]string(nil), base...)
	current[10], current[1750] = "first change", "second change"
	incoming[20], incoming[1700] = "third change", "fourth change"
	want[10], want[1750], want[20], want[1700] = current[10], current[1750], incoming[20], incoming[1700]
	if got := mergeMarkdown(strings.Join(base, "\n"), strings.Join(current, "\n"), strings.Join(incoming, "\n")); got != strings.Join(want, "\n") {
		t.Fatal("wide merge duplicated or dropped lines")
	}
}

func FuzzMergeMarkdown(f *testing.F) {
	f.Add("A\nB\nC\nD", "B\nD", "A\nnew\nB\nC!")
	f.Add("1\n2\n3\n4", "1\na\n4", "b\n3\nc")
	f.Fuzz(func(t *testing.T, base, current, incoming string) {
		if len(base)+len(current)+len(incoming) > 3000 {
			t.Skip()
		}
		got := mergeMarkdown(base, current, incoming)
		if got != mergeMarkdown(base, current, incoming) {
			t.Fatal("merge must be deterministic")
		}
		if mergeMarkdown(base, current, base) != current || mergeMarkdown(base, base, incoming) != incoming {
			t.Fatal("merge changed the only edited version")
		}
	})
}

func TestVaultSessionsAndConcurrentSaves(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI to run against MongoDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database(fmt.Sprintf("vault_test_%d", time.Now().UnixNano()))
	defer func() { _ = db.Drop(context.Background()) }()
	v, err := openVault(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	password := "integration test password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_PASSWORD_HASH", string(hash))
	t.Setenv("APP_USERNAME", "test")
	t.Setenv("APP_SESSION_SECURE", "true")
	auth, err := openAuthentication(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	v.routes(mux)
	server := httptest.NewTLSServer(auth.protect(mux))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := server.Client()
	httpClient.Timeout = 5 * time.Second
	httpClient.Jar = jar
	request := func(method, path string, body any, want int) ([]byte, []*http.Cookie, error) {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+"/api"+path, bytes.NewReader(payload))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := httpClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err == nil && res.StatusCode != want {
			err = fmt.Errorf("%s %s = %d, want %d: %s", method, path, res.StatusCode, want, data)
		}
		return data, res.Cookies(), err
	}
	check := func(method, path string, body any, want int, target any) []*http.Cookie {
		t.Helper()
		data, cookies, err := request(method, path, body, want)
		if err != nil {
			t.Fatal(err)
		}
		if target != nil {
			if err := json.Unmarshal(data, target); err != nil {
				t.Fatal(err)
			}
		}
		return cookies
	}
	credentials := map[string]string{"username": "test", "password": "wrong"}
	check("GET", "/vault", nil, 401, nil)
	check("POST", "/session", credentials, 401, nil)
	credentials["password"] = password
	cookies := check("POST", "/session", credentials, 200, nil)
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie settings")
	}
	var folder vaultFolder
	check("POST", "/vault/folders", map[string]string{"name": "Projects"}, 201, &folder)
	var document vaultContent
	check("POST", "/vault/documents", map[string]any{"title": "Notes", "folder_id": folder.ID,
		"markdown": "# Notes\ninitial\n"}, 201, &document)
	path := "/vault/documents/" + document.ID.Hex()

	// Three simultaneous writers start from the same immutable revision.
	var writers sync.WaitGroup
	for _, line := range []string{"alpha", "beta", "gamma"} {
		writers.Go(func() {
			_, _, err := request("PUT", path, map[string]any{"title": "Notes", "folder_id": folder.ID,
				"markdown": "# Notes\n" + line + "\n", "revision": document.Revision}, 200)
			if err != nil {
				t.Error(err)
			}
		})
	}
	writers.Wait()
	var saved vaultContent
	check("GET", path, nil, 200, &saved)
	lines := strings.Split(saved.Markdown, "\n")
	if len(lines) != 5 || lines[0] != "# Notes" || lines[4] != "" {
		t.Fatalf("unexpected merged document: %q", saved.Markdown)
	}
	for _, line := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(saved.Markdown, "\n"+line+"\n") {
			t.Errorf("merged document is missing %q", line)
		}
	}

	check("PUT", path, map[string]any{"title": "Notes", "folder_id": nil,
		"markdown": saved.Markdown, "revision": saved.Revision}, 200, nil)
	var list struct{ Documents []vaultDocument }
	check("GET", "/vault?folder="+folder.ID.Hex(), nil, 200, &list)
	if len(list.Documents) != 0 {
		t.Fatal("moved document remained in its previous folder")
	}
	check("GET", "/vault", nil, 200, &list)
	if len(list.Documents) != 1 || list.Documents[0].ID != document.ID {
		t.Fatal("moved document missing from root")
	}
	check("DELETE", "/session", nil, 204, nil)
	check("GET", path, nil, 401, nil)
}
