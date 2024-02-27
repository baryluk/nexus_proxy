package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func isUnsafeFilename(filename string) bool {
	return strings.HasPrefix(filename, "../") || strings.HasPrefix(filename, "/") || strings.HasSuffix(filename, "/..") || strings.HasSuffix(filename, "/") || strings.Contains(filename, "//") || strings.Contains(filename, "/../") || strings.Contains(filename, "/./") || strings.Contains(filename, "\\")
}

func proxyHandler(repos map[string]*Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requests_in_progress.Inc()
		defer func() {
			requests_in_progress.Dec()
		}()

		log.Printf("PRE %s   0 %q Request handler started\n", r.RemoteAddr, r.URL.Path)
		if r.Method != "GET" {
			error_count.Inc()
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			log.Printf("END %s 405 %q Method %q not allowed\n", r.RemoteAddr, r.Method, r.URL.Path)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/proxy/")
		// pathSplit := strings.SplitN(path, "/", 2)
		//if len(pathSplit) != 2 {

		reponame, filename, good := strings.Cut(path, "/")
		if !good {
			error_count.Inc()
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 Not Found\n\nNeed to provide repo name i.e. /proxy/myrepo/...\n"))
			log.Printf("END %s 404 %q Unsupported URL\n", r.RemoteAddr, path)
			return
		}
		// reponame := pathSplit[0]
		// filename := pathSplit[1]
		// fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))

		// Technically we do not need to check reponame for cache hit.
		// But: 1) This would allow to pass invalid repo name (possibly containing "/../")
		// 2) We might have per-repo state of in-progress requests to show on /status page.
		repo, ok := repos[reponame]
		if !ok {
			error_count.Inc()
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 Not Found\n\nRepo " + reponame + " not configured\n"))
			log.Printf("END %s 404 %q No coresponding repo %q\n", r.RemoteAddr, path, reponame)
			return
		}

		// Go http server automatically canonicalizes r.URL.Path, and rejects
		// queries that go higher in path hierarchy. But do extra checks just
		// just to be sure. (Original real URL can be found in r.URL.RawPath
		if isUnsafeFilename(filename) {
			error_count.Inc()
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("400 Bad Request\n\nProhobited byte sequence in filename\n"))
			log.Printf("END %s 400 %q Prohibited byte sequence in filename\n", r.RemoteAddr, path, reponame)
			return
		}

		cacheFilename := "cache/" + reponame + "/final/" + filename
		cache, err := os.Open(cacheFilename)
		// Cache hit
		if err == nil {
			defer cache.Close()
			handleHit(w, r, path, cache)
			return
		}

		handleMiss(w, r, reponame, repo, path, filename, cacheFilename)
	}
}

func handleHit(w http.ResponseWriter, r *http.Request, path string, cache *os.File) {
	hit_requests_in_progress.Inc()
	defer func() {
		hit_requests_in_progress.Dec()
	}()

	fi, err := cache.Stat()
	if err != nil {
		error_count.Inc()
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 Internal Server Error\n\nCould not call f.Stat() on cached file\n"))
		log.Printf("END %s 500 %q Failed to call f.Stat(). Error: %v", r.RemoteAddr, path, err)
		return
	}
	fileSize := fi.Size()
	log.Printf("MID %s 200 %q Cache hit, %d bytes - serving", r.RemoteAddr, path, fileSize)
	t1 := time.Now()
	w.Header().Set("Content-Length", strconv.Itoa(int(fileSize)))
	w.WriteHeader(http.StatusOK)

	// io.Copy uses 32KiB buffer by default if it needs to.
	// But also io.Copy for suitable files, uses Linux sendfile,
	// which is in this case, from file to unencrypted socket.
	// From strace it looks like it is sending in 4MiB chunks.
	bytesCopiedCount, err := io.Copy(w, cache)

	// buf := make([]byte, 1024*1024)
	// bytesCopiedCount, err := io.CopyBuffer(w, cache, buf)

	// n, err := syscall.Sendfile(int(w.Fd()), int(cache.Fd()), nil, int(fileSize))
	// if err == syscall.EAGAIN

	if err != nil {
		error_count.Inc()
		log.Printf("END %s   - %q Cache hit, %d bytes - premature error after %d / %d bytes in %v. Error: %v", r.RemoteAddr, path, fileSize, bytesCopiedCount, fileSize, time.Since(t1), err)
		panic(http.ErrAbortHandler)
	}
	// w.Close()
	flusher := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	_ = bytesCopiedCount
	log.Printf("END %s 200 %q Cache hit, %d bytes - served in %v", r.RemoteAddr, path, fileSize, time.Since(t1))
	hit_count.Inc()
	hit_bytes.Add(float64(bytesCopiedCount))
	return
}

const (
	BUFFERSIZE = 16 * 1024
)

func handleMiss(w http.ResponseWriter, r *http.Request, reponame string, repo *Repo, path, filename, cacheFilename string) {
	// Cache miss
	miss_requests_in_progress.Inc()
	defer func() {
		miss_requests_in_progress.Dec()
	}()
	miss_count.Inc()
	resp, err := http.Get(repo.upstreamURLBase + filename)
	if err != nil {
		upstream_error_count.Inc()
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 Internal Server Error\n\nProxy request " + filename + " failed\n"))
		log.Printf("END %s 500 %q Cache miss and upstream request error %v", r.RemoteAddr, path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		upstream_error_count.Inc()
		w.WriteHeader(resp.StatusCode)
		log.Printf("END %s %d %q Cache miss and upstream response error", r.RemoteAddr, resp.StatusCode, path)
		return
	}
	// w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	contentLength := resp.Header.Get("Content-Length")

	cacheTemp, err := NewTempFile("cache/"+reponame+"/temp", filename, cacheFilename)
	if err != nil {
		error_count.Inc()
		log.Printf("MID0 %s 500 %q Cache miss and fs error %v", r.RemoteAddr, path, err)

		// Fallback to streaming directly to user only.
		if contentLength != "" {
			w.Header().Set("Content-Length", contentLength)
		}
		w.WriteHeader(http.StatusOK)
		t1 := time.Now()
		bytesCopiedCount, err := io.Copy(w, resp.Body)
		if err != nil {
			upstream_error_count.Inc()
			error_count.Inc()
			log.Printf("END %s 5xx %q Writing to client socket or reading from upstream socket failed after %d bytes, aborting response. Error: %v", r.RemoteAddr, path, bytesCopiedCount, err)
			panic(http.ErrAbortHandler)
		}
		resp.Body.Close()
		log.Printf("END %s 2xx %q Finished streaming to client (without saving to cache due to previous errors). %d bytes in %v", r.RemoteAddr, path, bytesCopiedCount, time.Since(t1))
		miss_bytes.Add(float64(bytesCopiedCount))
		return
	}
	defer func() {
		err := cacheTemp.Cleanup()
		if err != nil {
			error_count.Inc()
			log.Printf("FIN %s   - %q Sending response or saving to cache filed, and temporary file cleanup failed. Error: %v", r.RemoteAddr, path, err)
		}
		update_free_disk_space()
	}()

	if contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}
	w.WriteHeader(http.StatusOK)
	log.Printf("MID %s 200 %q Cache miss - serving expected %q bytes from upstream", r.RemoteAddr, path, contentLength)

	// Note, we do not use io.MultiWriter, because in case writing to file fails,
	// we want to continue proxing the request.
	t1 := time.Now()
	bytesCopiedCount := 0
	buf := make([]byte, BUFFERSIZE)
	abandonCacheFile := false
	for {
		n, err := resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			upstream_error_count.Inc()
			panic(http.ErrAbortHandler)
		}
		if n == 0 {
			break
		}
		if _, err := w.Write(buf[:n]); err != nil {
			error_count.Inc()
			log.Printf("END %s 5xx %q Cache miss and writing to socket failed after %d bytes, aborting response and cache write. Error: %v", r.RemoteAddr, path, bytesCopiedCount, err)
			panic(http.ErrAbortHandler)
		}
		if !abandonCacheFile {
			if n2, err := cacheTemp.File().Write(buf[:n]); err != nil || n2 != n {
				error_count.Inc()
				log.Printf("MID0 %s 200 %q Cache miss and write error to cache file after %d bytes. Attempted to write %d bytes, wrote %d bytes. Error: %v", r.RemoteAddr, path, bytesCopiedCount, n, n2, err)
				abandonCacheFile = true
			}
		}
		bytesCopiedCount += n
	}

	resp.Body.Close()
	miss_bytes.Add(float64(bytesCopiedCount))

	if abandonCacheFile {
		log.Printf("END %s 2xx %q Finished streaming to client (writing to cache file aborted due to errors). %d bytes in %v", r.RemoteAddr, path, bytesCopiedCount, time.Since(t1))
	} else {
		log.Printf("END %s 2xx %q Finished streaming to client and to cache file. %d bytes in %v", r.RemoteAddr, path, bytesCopiedCount, time.Since(t1))

		lastSlash := strings.LastIndex(filename, "/")
		if lastSlash != -1 {
			err = os.MkdirAll("cache/"+reponame+"/final/"+filename[0:lastSlash], 0750)
			if err != nil {
				error_count.Inc()
				log.Printf("FIN %s   - %q Failed creating final subdirectory for cache file. Error: %v", r.RemoteAddr, path, err)
				return
			}
		}
		err = cacheTemp.Finalize()
		if err != nil {
			error_count.Inc()
			log.Printf("FIN %s   - %q Failed closing or moving temporary cache file. Error: %v", r.RemoteAddr, path, err)
		}
	}
}
