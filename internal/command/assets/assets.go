// Package assets implements `kamu assets` — upload org images to the Kamu CDN
// (served from files.kskamu.app) and check the org's storage quota. Auth is a
// kamuhub access key (a scoped, signed platform context); export
// KAMU_ACCESS_KEY or pass --key.
package assets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/assets"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

const (
	envKey = "KAMU_ACCESS_KEY"
	envURL = "KAMU_ASSETS_URL"

	// maxUploadBytes mirrors the server's cap. Checked client-side only to fail
	// fast with a clearer message — the server still enforces it.
	maxUploadBytes = 5 * 1024 * 1024
)

// allowedExts mirrors the image types the server accepts. Client-side it's a
// pre-check on the file name; the server decides by sniffing the bytes.
var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

func New() *cobra.Command {
	cmd := command.New("assets", "Manage org images on the Kamu CDN", "", nil)
	cmd.AddCommand(newUpload())
	cmd.AddCommand(newUsage())
	return cmd
}

// resolveKey pulls the access key from --key or the env, with the same guidance
// the other product commands give.
func resolveKey(key string) (string, error) {
	k := config.ResolveAccessKey(key)
	if k == "" {
		return "", errors.New("no access key. Run `kamu login`, or export " + envKey + "=... or pass --key <token>")
	}
	return k, nil
}

// resolveOrg picks the org slug to send: the --org flag, else the single org in
// the access key payload. An opaque/unreadable key sends no org and lets the
// server resolve a single-org context itself; a key visibly spanning several
// orgs errors here — the server would 400 anyway, this is just a clearer
// message.
func resolveOrg(flag, key string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	slugs := keyOrgSlugs(key)
	switch len(slugs) {
	case 0:
		return "", nil
	case 1:
		return slugs[0], nil
	default:
		return "", fmt.Errorf("the access key covers several orgs (%s); pass --org <slug>", strings.Join(slugs, ", "))
	}
}

// keyOrgSlugs reads the org slugs from the access key payload (no verification
// — the server verifies; we only need to route).
func keyOrgSlugs(key string) []string {
	parts := strings.Split(key, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var p struct {
		Orgs []struct {
			Slug string `json:"slug"`
		} `json:"orgs"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	var slugs []string
	for _, o := range p.Orgs {
		if o.Slug != "" {
			slugs = append(slugs, o.Slug)
		}
	}
	return slugs
}

// validateUpload is the client-side pre-check: right extension, under the size
// cap. Clearer than a server round-trip for the obvious mistakes; the server
// still decides by sniffing the bytes.
func validateUpload(path string, size int64) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedExts[ext] {
		return fmt.Errorf("unsupported file type %q (want jpg, jpeg, png, webp or gif)", ext)
	}
	if size > maxUploadBytes {
		return fmt.Errorf("file is %s — over the 5 MB limit", humanBytes(size))
	}
	return nil
}

// uploadRow is one file's outcome for --json output: the local path plus the
// server's stored-image record.
type uploadRow struct {
	File string `json:"file"`
	*assets.UploadResult
}

func newUpload() *cobra.Command {
	var key, org string
	var asJSON bool
	cmd := command.New("upload <file>...", "Upload images — deduplicated, served from files.kskamu.app", "", func(ctx context.Context, args []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		o, err := resolveOrg(org, k)
		if err != nil {
			return err
		}
		client := assets.New(os.Getenv(envURL), k)

		var failed int
		var rows [][]string
		results := []uploadRow{}
		for _, path := range args {
			res, err := uploadOne(ctx, client, o, path)
			if err != nil {
				failed++
				fmt.Fprintf(io.ErrOut, "%s: %v\n", path, err)
				continue
			}
			note := ""
			if res.Existing {
				note = "(already uploaded)"
			}
			rows = append(rows, []string{path, res.URL, note})
			results = append(results, uploadRow{File: path, UploadResult: res})
		}
		if asJSON {
			if err := render.JSON(io.Out, results); err != nil {
				return err
			}
		} else if len(rows) > 0 {
			if err := render.Table(io.Out, nil, rows); err != nil {
				return err
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d uploads failed", failed, len(args))
		}
		return nil
	})
	cmd.Args = cobra.MinimumNArgs(1)
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&org, "org", "", "org slug (defaults to the key's org)")
	f.BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func uploadOne(ctx context.Context, client *assets.Client, org, path string) (*assets.UploadResult, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if err := validateUpload(path, fi.Size()); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return client.Upload(ctx, org, data)
}

func newUsage() *cobra.Command {
	var key, org string
	var asJSON bool
	cmd := command.New("usage", "Show the org's image count and storage quota", "", func(ctx context.Context, _ []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		o, err := resolveOrg(org, k)
		if err != nil {
			return err
		}
		client := assets.New(os.Getenv(envURL), k)

		u, err := client.Usage(ctx, o)
		if err != nil {
			return err
		}
		if asJSON {
			return render.JSON(io.Out, u)
		}
		percent := "n/a"
		if u.LimitBytes > 0 {
			percent = fmt.Sprintf("%.1f%%", 100*float64(u.Usage.Bytes)/float64(u.LimitBytes))
		}
		return render.Table(io.Out, nil, [][]string{
			{"Org", u.Org},
			{"Images", strconv.FormatInt(u.Usage.Count, 10)},
			{"Used", fmt.Sprintf("%s (%d bytes)", humanBytes(u.Usage.Bytes), u.Usage.Bytes)},
			{"Limit", humanBytes(u.LimitBytes)},
			{"Used %", percent},
		})
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&org, "org", "", "org slug (defaults to the key's org)")
	f.BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

// humanBytes renders a byte count in the binary units the quota is set in
// (5 MB cap = 5*1024*1024).
func humanBytes(n int64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f kB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
