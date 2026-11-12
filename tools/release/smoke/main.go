// Command smoke exercises the real updater against a signed release in an isolated app bundle.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

type source struct{ url string }

func (s source) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	return s.Open(ctx, s.url+"/manifest")
}

func (s source) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("update server returned %d", response.StatusCode)
	}
	return response.Body, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir := os.Args[1]
	data, err := os.ReadFile(filepath.Join(dir, "update-darwin-arm64.json"))
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	executable, _ := os.Executable()
	original, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	signature := manifest["signature"]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest" {
			_ = json.NewEncoder(w).Encode(manifest)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "repsync-"+manifest["version"].(string)+"-darwin-arm64.tar.gz"))
	}))
	defer server.Close()
	manifest["url"] = server.URL + "/archive"
	mygo.App.SetVersion("0.0.0")
	mygo.Updater.SetSource(source{server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manifest["signature"] = base64.StdEncoding.EncodeToString(make([]byte, 64))
	update, err := mygo.Updater.Check(ctx)
	if err != nil || update == nil {
		return fmt.Errorf("checking tampered update: %v", err)
	}
	if err := update.Install(ctx, nil); err == nil {
		return fmt.Errorf("tampered signature was accepted")
	}
	current, _ := os.ReadFile(executable)
	if sha256.Sum256(current) != sha256.Sum256(original) {
		return fmt.Errorf("refused update changed the original app")
	}
	manifest["signature"] = signature
	update, err = mygo.Updater.Check(ctx)
	if err != nil || update == nil {
		return fmt.Errorf("checking signed update: %v", err)
	}
	if err := update.Install(ctx, nil); err != nil {
		return err
	}
	current, _ = os.ReadFile(executable)
	if sha256.Sum256(current) != sha256.Sum256(original) {
		return fmt.Errorf("app was replaced before its process exited")
	}
	fmt.Println("Rejected invalid signature; staged valid update without replacing the running app")
	return nil
}
