package main

import (
	"flag"
	"net/http"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/liuzl/goutil"
	"github.com/liuzl/goutil/rest"
	"github.com/liuzl/store"
	"github.com/rs/zerolog/hlog"
)

var (
	addr       = flag.String("addr", ":8080", "bind address")
	dbDir      = flag.String("db", "./db", "db dir")
	levelStore *store.LevelStore
	onceStore  sync.Once
	n          uint64 = 10000
	mu         sync.Mutex
)

type Record struct {
	Url string `json:"url"`
	Ext string `json:"ext"`
}

func GetLevelStore() *store.LevelStore {
	onceStore.Do(func() {
		var err error
		var b []byte
		if levelStore, err = store.NewLevelStore(*dbDir); err != nil {
			panic(err)
		}
		if b, err = levelStore.Get("meta:total"); err == nil {
			if err = store.BytesToObject(b, &n); err != nil {
				panic(err)
			}
			return
		}
		if err.Error() != "leveldb: not found" {
			panic(err)
		}
	})
	return levelStore
}

func CHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infof("addr=%s  method=%s host=%s uri=%s",
		r.RemoteAddr, r.Method, r.Host, r.RequestURI)
	r.ParseForm()
	url := strings.TrimSpace(r.FormValue("url"))
	ext := strings.TrimSpace(r.FormValue("ext"))
	if url == "" {
		rest.MustEncode(w, rest.RestMessage{"error", "url is empty"})
		return
	}
	rec := &Record{url, ext}
	key := "\t" + url + "\t" + ext
	b, err := GetLevelStore().Get(key)
	if err == nil {
		rest.MustEncode(w, rest.RestMessage{"ok", map[string]interface{}{
			"code": string(b), "info": rec, "new": false}})
		return
	}
	if err.Error() != "leveldb: not found" {
		rest.MustEncode(w, rest.RestMessage{"error", err.Error()})
		return
	}
	mu.Lock()
	defer mu.Unlock()
	code := goutil.ToB62(n)
	GetLevelStore().Put(key, []byte(code))
	bytes, _ := store.ObjectToBytes(rec)
	GetLevelStore().Put(code, bytes)
	n += 1
	rest.MustEncode(w, rest.RestMessage{"ok", map[string]interface{}{
		"code": code, "info": rec, "new": true}})
}

func SaveHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infof("addr=%s  method=%s host=%s uri=%s",
		r.RemoteAddr, r.Method, r.Host, r.RequestURI)
	mu.Lock()
	defer mu.Unlock()
	bytes, err := store.ObjectToBytes(n)
	if err != nil {
		rest.MustEncode(w, rest.RestMessage{"error", err.Error()})
		return
	}
	GetLevelStore().Put("meta:total", bytes)
	rest.MustEncode(w, rest.RestMessage{"ok", "saved"})

}

func NHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infof("addr=%s  method=%s host=%s uri=%s",
		r.RemoteAddr, r.Method, r.Host, r.RequestURI)
	rest.MustEncode(w, rest.RestMessage{"ok", n})
}

func MainHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infof("addr=%s  method=%s host=%s uri=%s",
		r.RemoteAddr, r.Method, r.Host, r.RequestURI)
	code := r.URL.Path[1:]
	if b, err := GetLevelStore().Get(code); err != nil {
		rest.MustEncode(w, rest.RestMessage{"error", err.Error()})
	} else {
		rec := new(Record)
		if err = store.BytesToObject(b, rec); err != nil {
			rest.MustEncode(w, rest.RestMessage{"error", err.Error()})
		} else {
			hlog.FromRequest(r).Info().
				Str("code", code).Str("url", rec.Url).Str("ext", rec.Ext).
				Msg("redirect")
			http.Redirect(w, r, rec.Url, 301)
		}
	}
}

func main() {
	flag.Parse()
	defer glog.Flush()
	defer glog.Info("server exit")
	GetLevelStore()
	http.Handle("/c/", rest.WithLog(CHandler))
	http.Handle("/n/", rest.WithLog(NHandler))
	http.Handle("/save/", rest.WithLog(SaveHandler))
	http.Handle("/", rest.WithLog(MainHandler))
	glog.Info("server listen on", *addr)
	glog.Error(http.ListenAndServe(*addr, nil))
}
