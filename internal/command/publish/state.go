package publish

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kotisivukamu/kamucli/internal/config"
)

// Local publish state: which forge repo a given local file was published to.
//
// Republishing has to find the SAME repo, or every `kamu publish page.html`
// would mint a new repo and a new URL — and the whole point of repo-per-file is
// that the link and the history are stable. The mapping has to live somewhere,
// and the two obvious places are worse: a dotfile beside the user's document
// litters their directory (and gets committed, or shared, or deleted), and
// deriving the repo from the filename would collide across directories and
// break on rename. So it goes in the CLI's own state dir, alongside the config
// it already owns, keyed by the file's absolute path.
//
// It is a cache, not a source of truth: `--repo owner/name` republishes to any
// repo regardless of what is recorded here, so losing the file (new machine,
// wiped home dir) costs a flag, not the page.

type stateFile struct {
	// Absolute local path -> the publish it belongs to.
	Files map[string]Record `json:"files"`
}

// Record is what the CLI needs to republish without asking kamuhub again.
type Record struct {
	ID        string `json:"id"`
	Org       string `json:"org"`
	Repo      string `json:"repo"` // owner/name
	CloneURL  string `json:"clone_url"`
	Branch    string `json:"default_branch"`
	Path      string `json:"path"`
	PublicURL string `json:"public_url"`
}

// stateDir is the CLI's state directory — the same ~/.kamu (or $KAMU_CONFIG's
// directory) the config lives in, so a user with a relocated config gets their
// publish state relocated with it.
func stateDir() (string, error) {
	p, err := config.DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

func statePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "publishes.json"), nil
}

func loadState() (*stateFile, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &stateFile{Files: map[string]Record{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s stateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = map[string]Record{}
	}
	return &s, nil
}

// saveState writes the map atomically at 0600 in a 0700 dir, matching how the
// config is written — the records name private repos, so they are not world
// readable.
func saveState(s *stateFile) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".publishes-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// lookupByRepo finds a recorded publish by its owner/name, so `--repo` can reuse
// the known clone URL and target path instead of guessing them.
func (s *stateFile) lookupByRepo(repo string) (Record, bool) {
	for _, r := range s.Files {
		if strings.EqualFold(r.Repo, repo) {
			return r, true
		}
	}
	return Record{}, false
}

// workdir is where the CLI keeps its working clone of a publish repo. Cached
// under the state dir so a republish is a fetch, not a fresh clone, and so the
// user's own directories stay free of a git repo they never asked for.
func workdir(repo string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	owner, name, _ := strings.Cut(repo, "/")
	return filepath.Join(dir, "publish", owner, name), nil
}
