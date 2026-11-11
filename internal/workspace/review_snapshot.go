package workspace

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ReviewFile struct {
	Path      string `json:"path"`
	Index     string `json:"index"`
	Worktree  string `json:"worktree"`
	Untracked bool   `json:"untracked"`
	Conflict  bool   `json:"conflict"`
}

type ReviewSnapshot struct {
	Head             string       `json:"head"`
	Branch           string       `json:"branch"`
	Fingerprint      string       `json:"fingerprint"`
	FingerprintError string       `json:"fingerprintError"`
	Files            []ReviewFile `json:"files"`
	Staged           int          `json:"staged"`
	Unstaged         int          `json:"unstaged"`
	Untracked        int          `json:"untracked"`
	Conflicts        int          `json:"conflicts"`
	Stashes          []GitStash   `json:"stashes"`
	Upstream         string       `json:"upstream"`
	Ahead            int          `json:"ahead"`
	Behind           int          `json:"behind"`
	UpstreamKnown    bool         `json:"upstreamKnown"`
	FetchedAt        string       `json:"fetchedAt"`
	Error            string       `json:"error"`
}

func reviewSnapshot(path string) ReviewSnapshot {
	snapshot := ReviewSnapshot{Files: []ReviewFile{}, Stashes: []GitStash{}}
	stashes, err := readGitStashes(path)
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	snapshot.Head, snapshot.Branch, snapshot.Stashes = stashes.Head, stashes.Branch, stashes.Entries
	raw, err := git(path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil || strings.Contains(raw, "[output exceeds 1 MiB") {
		snapshot.Error = "Could not read the full workspace file state"
		return snapshot
	}
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			snapshot.Error = "Could not parse workspace file state"
			return snapshot
		}
		file := ReviewFile{Path: entry[3:], Index: entry[:1], Worktree: entry[1:2], Untracked: entry[:2] == "??"}
		file.Conflict = strings.Contains(entry[:2], "U") || entry[:2] == "AA" || entry[:2] == "DD"
		if file.Untracked {
			snapshot.Untracked++
		} else {
			if file.Index != " " {
				snapshot.Staged++
			}
			if file.Worktree != " " {
				snapshot.Unstaged++
			}
		}
		if file.Conflict {
			snapshot.Conflicts++
		}
		snapshot.Files = append(snapshot.Files, file)
		if strings.ContainsAny(entry[:2], "RC") {
			i++
		}
	}
	upstream, err := git(path, "rev-parse", "--symbolic-full-name", "@{upstream}")
	if err == nil {
		snapshot.Upstream = strings.TrimSpace(upstream)
		counts, e := git(path, "rev-list", "--left-right", "--count", "HEAD..."+snapshot.Upstream)
		values := strings.Fields(counts)
		if e == nil && len(values) == 2 {
			a, ae := strconv.Atoi(values[0])
			b, be := strconv.Atoi(values[1])
			if ae == nil && be == nil {
				snapshot.Ahead, snapshot.Behind, snapshot.UpstreamKnown = a, b, true
			}
		}
	}
	if fetch, e := git(path, "rev-parse", "--path-format=absolute", "--git-path", "FETCH_HEAD"); e == nil {
		if info, e := os.Stat(strings.TrimSpace(fetch)); e == nil {
			snapshot.FetchedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
	}
	snapshot.Fingerprint, err = reviewFingerprint(path)
	if err != nil {
		snapshot.FingerprintError = err.Error()
	}
	return snapshot
}

// reviewFingerprint hashes actual tracked/untracked contents, not timestamps or stash data.
// Staging and committing unchanged contents do not invalidate file-based checks.
func reviewFingerprint(path string) (string, error) {
	top, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not a readable Git worktree")
	}
	top = strings.TrimSpace(top)
	raw, err := git(top, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil || strings.Contains(raw, "[output exceeds 1 MiB") {
		return "", errors.New("could not list all files for review")
	}
	files := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	slices.Sort(files)
	files = slices.Compact(files)
	if len(files) > 5000 {
		return "", errors.New("more than 5000 files; content consistency not verified")
	}
	root, err := os.OpenRoot(top)
	if err != nil {
		return "", err
	}
	defer root.Close()
	hash := sha256.New()
	var total int64
	deadline := time.Now().Add(10 * time.Second)
	for _, name := range files {
		if name == "" {
			continue
		}
		if !filepath.IsLocal(name) || time.Now().After(deadline) {
			return "", errors.New("could not confirm file path or read deadline")
		}
		info, e := root.Lstat(name)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return "", fmt.Errorf("read file metadata %s: %w", name, e)
		}
		fmt.Fprintf(hash, "%q:%d:", name, info.Mode()&(os.ModeType|0111))
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := root.Readlink(name)
			if e != nil {
				return "", e
			}
			fmt.Fprintf(hash, "%q\n", target)
			continue
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("%s is not a regular file; verify submodules separately", name)
		}
		if total+info.Size() > 40*1024*1024 {
			return "", errors.New("total file size exceeds 40 MiB; content consistency not verified")
		}
		file, e := root.Open(name)
		if e != nil {
			return "", e
		}
		content := sha256.New()
		n, readErr := io.Copy(content, io.LimitReader(file, 40*1024*1024-total+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return "", errors.Join(readErr, closeErr)
		}
		total += n
		if total > 40*1024*1024 {
			return "", errors.New("file size limit exceeded while reading")
		}
		current, e := root.Lstat(name)
		if e != nil || !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
			return "", errors.New("file changed while reading; check again")
		}
		fmt.Fprintf(hash, "%x\n", content.Sum(nil))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
