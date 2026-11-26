package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type fstate struct {
	exists bool
	data   []byte
}

type snapshot map[string]fstate

// target is a path to read, with the directory the symlink check starts from.
// skipBase leaves base itself unchecked (the check starts one component below).
type target struct {
	path, base string
	skipBase   bool
}

type op struct {
	path   string
	data   []byte
	remove bool
}

// checkLinks refuses a symlink or junction at any component from base
// (inclusive unless skipBase) down to path.
func checkLinks(base, path string, skipBase bool) error {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	cur := base
	parts := []string{""}
	if skipBase {
		parts = nil
	}
	if rel != "." {
		parts = append(parts, strings.Split(rel, string(filepath.Separator))...)
	}
	for _, p := range parts {
		cur = filepath.Join(cur, p)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking %s: %w", cur, err)
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return &SymlinkError{Path: cur}
		}
	}
	return nil
}

func readSnap(ts []target) (snapshot, error) {
	s := snapshot{}
	for _, t := range ts {
		if err := checkLinks(t.base, t.path, t.skipBase); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(t.path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			s[t.path] = fstate{}
		case err != nil:
			return nil, fmt.Errorf("reading %s: %w", t.path, err)
		default:
			s[t.path] = fstate{exists: true, data: b}
		}
	}
	return s, nil
}

func diffSnap(a, b snapshot) []Conflict {
	var out []Conflict
	for p, x := range a {
		y := b[p]
		if x.exists != y.exists || !bytes.Equal(x.data, y.data) {
			out = append(out, Conflict{Path: p, Reason: "changed while installing"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

type undoRec struct {
	path    string
	existed bool
	data    []byte
	mode    fs.FileMode
}

type txn struct {
	opts *Options
	undo []undoRec
	dirs []string // directories this run created, outermost first
}

func (t *txn) mkdirAll(dir string) error {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil {
			break
		}
		missing = append(missing, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		t.dirs = append(t.dirs, missing[i])
	}
	return nil
}

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".rig-crew-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp) // best effort; the write already failed
	}
	return err
}

func (t *txn) apply(o op) error {
	if t.opts.beforeWrite != nil {
		if err := t.opts.beforeWrite(o.path); err != nil {
			return err
		}
	}
	u := undoRec{path: o.path, mode: 0o644}
	if fi, err := os.Stat(o.path); err == nil {
		b, rerr := os.ReadFile(o.path)
		if rerr != nil {
			return fmt.Errorf("reading %s: %w", o.path, rerr)
		}
		u.existed, u.data, u.mode = true, b, fi.Mode().Perm()
	}
	t.undo = append(t.undo, u)
	if o.remove {
		if err := os.Remove(o.path); err != nil {
			return fmt.Errorf("removing %s: %w", o.path, err)
		}
		return nil
	}
	if err := t.mkdirAll(filepath.Dir(o.path)); err != nil {
		return err
	}
	if err := writeAtomic(o.path, o.data, u.mode); err != nil {
		return fmt.Errorf("writing %s: %w", o.path, err)
	}
	return nil
}

// rollback restores every path touched, newest first.
func (t *txn) rollback() error {
	var first error
	for i := len(t.undo) - 1; i >= 0; i-- {
		u := t.undo[i]
		var err error
		if u.existed {
			err = writeAtomic(u.path, u.data, u.mode)
		} else if rerr := os.Remove(u.path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = rerr
		}
		if err != nil && first == nil {
			first = fmt.Errorf("restoring %s: %w", u.path, err)
		}
	}
	return first
}

func (t *txn) removeCreatedDirs() {
	for i := len(t.dirs) - 1; i >= 0; i-- {
		_ = os.Remove(t.dirs[i]) // fails when not empty, which is the point
	}
}

// lock takes the exclusive lock file under dir/.rig-crew.
func (t *txn) lock(dir string) (func(), error) {
	if err := t.mkdirAll(filepath.Join(dir, stateDir)); err != nil {
		return nil, err
	}
	lp := lockPath(dir)
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil, &LockedError{Path: lp}
	}
	if err != nil {
		return nil, fmt.Errorf("locking %s: %w", lp, err)
	}
	_ = f.Close() // an empty file; closing cannot lose data
	// A failed remove leaves a stale lock; the next run reports it as LockedError naming the path.
	return func() { _ = os.Remove(lp) }, nil
}

type outcome struct {
	actions []Action
	ops     []op
	cleanup []string // directories to remove when empty, after the run
	prune   []string // removed files whose empty parents go too
	root    string
}

// lockDirs lists, in sorted order, the workflow directories some op writes under.
func lockDirs(dirs []string, ops []op) []string {
	var out []string
	for _, d := range dirs {
		for _, o := range ops {
			if strings.HasPrefix(o.path, d+string(filepath.Separator)) {
				out = append(out, d)
				break
			}
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

type job struct {
	root    string
	wfDirs  []string // workflow directories that may be written
	targets []target
	compute func(snapshot) (*outcome, error)
}

func run(ctx context.Context, j job, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	snap, err := readSnap(j.targets)
	if err != nil {
		return Result{}, err
	}
	out, err := j.compute(snap)
	if err != nil {
		return Result{}, err
	}
	res := Result{Actions: out.actions}
	for _, a := range out.actions {
		if a.Kind != KindUnchanged {
			res.Changed = true
		}
	}
	if len(out.ops) > 0 {
		res.Changed = true // a manifest or reference changed even if no path did
	}
	if res.Actions == nil {
		res.Actions = []Action{}
	}
	if opts.DryRun || len(out.ops) == 0 {
		return res, nil
	}
	tx := &txn{opts: &opts}
	release, err := tx.lock(j.root)
	if err != nil {
		tx.removeCreatedDirs()
		return Result{}, err
	}
	releases := []func(){release}
	fail := func(err error) (Result, error) {
		if rerr := tx.rollback(); rerr != nil {
			err = errors.Join(err, rerr)
		}
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
		tx.removeCreatedDirs()
		return Result{}, err
	}
	for _, d := range lockDirs(j.wfDirs, out.ops) {
		r, err := tx.lock(d)
		if err != nil {
			return fail(err)
		}
		releases = append(releases, r)
	}
	if opts.beforeRecheck != nil {
		opts.beforeRecheck()
	}
	snap2, err := readSnap(j.targets)
	if err != nil {
		return fail(err)
	}
	if c := diffSnap(snap, snap2); len(c) > 0 {
		return fail(&ConflictError{Conflicts: c})
	}
	for _, o := range out.ops {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if err := tx.apply(o); err != nil {
			return fail(err)
		}
	}
	for i := len(releases) - 1; i >= 0; i-- {
		releases[i]()
	}
	for _, p := range out.prune {
		for d := filepath.Dir(p); d != out.root && strings.HasPrefix(d, out.root); d = filepath.Dir(d) {
			if os.Remove(d) != nil { // not empty: stop
				break
			}
		}
	}
	for _, d := range out.cleanup {
		_ = os.Remove(d) // not empty means something else lives there
	}
	return res, nil
}
