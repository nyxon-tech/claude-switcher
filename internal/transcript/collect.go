package transcript

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// cacheVersion changes whenever record or cachedFile change shape; an old cache is then ignored.
const cacheVersion = 1

// record is one API message as it counts toward usage.
type record struct {
	ID    string
	Model string
	Time  time.Time
	Cwd   string
	Usage Usage // Messages is 1
}

type cachedFile struct {
	Size    int64
	ModTime time.Time
	Records []record // deduplicated within the file
}

type usageCache struct {
	Version int
	Files   map[string]cachedFile // by path
}

// Collect adds up token usage over every transcript, main and subagent files, counting each
// API message once (by message.id) and skipping "<synthetic>". cacheFile keeps per-file results
// between runs ("" disables the cache), so only changed files are parsed again. The cache is
// best effort: failing to save it costs speed next time, not this run's Stats. progress may be nil.
func Collect(projectsDir, cacheFile string, progress func(done, total int)) (Stats, error) {
	files, err := jsonlFiles(projectsDir)
	if err != nil {
		return Stats{}, err
	}
	report := func(done int) {
		if progress != nil {
			progress(done, len(files))
		}
	}
	old := loadCache(cacheFile)
	cur := make(map[string]cachedFile, len(files))
	var stale []fileStat
	for _, f := range files {
		if c, ok := old[f.path]; ok && c.Size == f.size && c.ModTime.Equal(f.modTime) {
			cur[f.path] = c
		} else {
			stale = append(stale, f)
		}
	}
	done := len(cur)
	if done > 0 {
		report(done)
	}
	recs, err := parseFiles(stale, func() { done++; report(done) })
	if err != nil {
		return Stats{}, err
	}
	for i, f := range stale {
		cur[f.path] = cachedFile{Size: f.size, ModTime: f.modTime, Records: recs[i]}
	}
	st := Stats{FilesRead: len(stale), FilesTotal: len(files)}
	aggregate(&st, projectsDir, files, cur)
	if cacheFile != "" && (len(stale) > 0 || len(old) != len(cur)) {
		_ = saveCache(cacheFile, cur) // e.g. an antivirus holding the old file on Windows
	}
	return st, nil
}

// parseFiles reads the records of files in parallel. done runs on the caller's goroutine after
// each file.
func parseFiles(files []fileStat, done func()) ([][]record, error) {
	recs := make([][]record, len(files))
	errs := make([]error, len(files))
	next := make(chan int)
	finished := make(chan struct{})
	for range min(runtime.GOMAXPROCS(0), len(files)) {
		go func() {
			for i := range next {
				recs[i], errs[i] = fileRecords(files[i].path)
				finished <- struct{}{}
			}
		}()
	}
	go func() {
		for i := range files {
			next <- i
		}
		close(next)
	}()
	for range files {
		<-finished
		done()
	}
	return recs, errors.Join(errs...)
}

type fileStat struct {
	path    string
	size    int64
	modTime time.Time
}

// jsonlFiles lists every .jsonl file under dir in a stable order.
func jsonlFiles(dir string) ([]fileStat, error) {
	var out []fileStat
	// The trailing separator makes WalkDir follow dir when it is a symlink or a junction.
	err := filepath.WalkDir(filepath.Clean(dir)+string(filepath.Separator), func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // the folder is missing or went away during the walk
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, fileStat{path, info.Size(), info.ModTime()})
		return nil
	})
	return out, err
}

// usageLine is the part of an assistant line that usage needs.
type usageLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

var assistantTag = []byte(`"type":"assistant"`)

// fileRecords reads the API messages of one file, each once.
func fileRecords(path string) ([]record, error) {
	f, err := openShared(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // deleted since the walk
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []record
	seen := map[string]bool{}
	err = scanLines(newReader(f), func(b []byte) bool {
		if !bytes.Contains(b, assistantTag) { // cheap skip of tool results and metadata
			return true
		}
		var l usageLine
		if json.Unmarshal(b, &l) != nil || l.Type != "assistant" || l.Message.Model == synthetic {
			return true
		}
		id := cmp.Or(l.Message.ID, l.RequestID)
		if id != "" && seen[id] {
			return true
		}
		seen[id] = true
		t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
		u := l.Message.Usage
		recs = append(recs, record{id, l.Message.Model, t, l.Cwd, Usage{u.Input, u.Output, u.CacheWrite, u.CacheRead, 1}})
		return true
	})
	return recs, err
}

// aggregate sums the records of every file, counting a message id once across files.
func aggregate(st *Stats, projectsDir string, files []fileStat, cache map[string]cachedFile) {
	st.ByModel, st.ByDay, st.ByProject = map[string]Usage{}, map[string]Usage{}, map[string]Usage{}
	seen := map[string]bool{}
	for _, f := range files {
		recs := cache[f.path].Records
		if len(recs) > 0 && isMain(projectsDir, f.path) {
			st.Sessions++
		}
		for _, r := range recs {
			if r.ID != "" && seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			st.ByModel[r.Model] = st.ByModel[r.Model].Add(r.Usage)
			st.ByProject[r.Cwd] = st.ByProject[r.Cwd].Add(r.Usage)
			if r.Time.IsZero() {
				continue
			}
			day := r.Time.Local().Format(time.DateOnly)
			st.ByDay[day] = st.ByDay[day].Add(r.Usage)
			if st.First.IsZero() || r.Time.Before(st.First) {
				st.First = r.Time
			}
			if r.Time.After(st.Last) {
				st.Last = r.Time
			}
		}
	}
}

// isMain reports whether path is a main transcript, <projects>/<slug>/<uuid>.jsonl.
func isMain(projectsDir, path string) bool {
	rel, err := filepath.Rel(projectsDir, path)
	if err != nil {
		return false
	}
	slug, name, ok := strings.Cut(filepath.ToSlash(rel), "/")
	_, isSession := sessionID(name)
	return ok && slug != ".." && isSession
}

func loadCache(path string) map[string]cachedFile {
	f, err := os.Open(path) // fails for "" too, which disables the cache
	if err != nil {
		return nil
	}
	defer f.Close()
	var c usageCache
	if gob.NewDecoder(bufio.NewReader(f)).Decode(&c) != nil || c.Version != cacheVersion {
		return nil
	}
	return c.Files
}

// saveCache writes the cache through a temp file and a rename, so a crash never leaves it torn.
func saveCache(path string, files map[string]cachedFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(tmp)
	err = gob.NewEncoder(bw).Encode(usageCache{cacheVersion, files})
	if err == nil {
		err = bw.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
