package main

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func startGCLoop(repos map[string]*Repo) chan bool {
	fileCount := 0
	dirCount := 0
	removedCount := 0
	bytesSum := int64(0)
	removedBytesSum := int64(0)

	walkerFactory := func(repo *Repo) func(path string, d fs.DirEntry, err error) error {
		return func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				dirCount++
				return nil
			}
			basename := d.Name()
			fi, err := d.Info()
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				log.Printf("gc: Walker: path: %q  dir: %v  Error getting Info: %v", path, d.IsDir(), err)
				return err
			}
			fileCount++
			// &os.fileStat{
			//   name:"foo2",
			//   size:0,
			//   mode:0x1a4,
			//   modTime:time.Time{wall:0x0, ext:63794987079, loc:(*time.Location)(0xc2a2e0)},
			//   sys:syscall.Stat_t{Dev:0x31, Ino:0x51f, Nlink:0x1, Mode:0x81a4,
			//                      Uid:0x3e8, Gid:0x3e8, X__pad0:0, Rdev:0x0,
			//                      Size:0, Blksize:4096, Blocks:0,
			//                      Atim:syscall.Timespec{Sec:1659390279, Nsec:0},
			//                      Mtim:syscall.Timespec{Sec:1659390279, Nsec:0},
			//                      Ctim:syscall.Timespec{Sec:1659390279, Nsec:0}, X__unused:[3]int64{0, 0, 0}}}
			_ = basename
			delete := true
			mtime := fi.ModTime()
			if !(time.Since(mtime) > repo.gcMaxAge) {
				delete = false
			}
			stat := fi.Sys().(*syscall.Stat_t)
			var atime, ctime time.Time
			if stat != nil {
				atime = time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec))
				if !(time.Since(atime) > repo.gcMaxAge) {
					delete = false
				}
				ctime = time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
				if !(time.Since(ctime) > repo.gcMaxAge) {
					delete = false
				}
			}
			if delete {
				log.Printf("gc: Walker: path: %q  dir: %v  mtime %s (%s ago)  atime %s (%s ago)  ctime %s (%s ago) - REMOVING", path, d.IsDir(), mtime, time.Since(mtime), atime, time.Since(atime), ctime, time.Since(ctime))
				err = os.Remove(path)
				if err != nil {
					gc_error_count.Inc()
					log.Printf("gc: Walker: path: %q Error, while removing: %v", err)
					bytesSum += fi.Size()
				} else {
					removedBytesSum += fi.Size()
					removedCount++
				}
				return nil
			} else {
				bytesSum += fi.Size()
				// log.Printf("gc: Walker: path: %q  dir: %v  mtime %s (%s ago)  atime %s (%s ago)  ctime %s (%s ago) - KEEPING", path, d.IsDir(), mtime, time.Since(mtime), atime, time.Since(atime), ctime, time.Since(ctime))
			}
			return nil
		}
	}

	updater := func(reponame string, repo *Repo) {
		if repo.gcMaxAge == 0 {
			log.Printf("gc: gc skipped for %s", reponame)
			return
		}
		log.Printf("gc: gc started for %s", reponame)

		t1 := time.Now()

		timer := prometheus.NewTimer(gc_loop_time)
		defer timer.ObserveDuration()
		gc_in_progress.Set(1)
		defer func() {
			last_gc_time.Set(time.Since(t1).Seconds())
			gc_in_progress.Set(0)
		}()

		fileCount = 0
		removedCount = 0
		dirCount = 0
		bytesSum = 0
		removedBytesSum = 0
		if err := filepath.WalkDir("cache/"+reponame, walkerFactory(repo)); err != nil {
			log.Printf("gc: Error walking cache: %v", err)
		}
		gc_final_size.Set(float64(bytesSum))
		gc_final_count.Set(float64(fileCount - removedCount))

		update_free_disk_space()
		log.Printf("gc: gc finished scanning %d directories and %d files (%d bytes remaining) in %s. %d files (%d bytes) removed.", dirCount, fileCount, bytesSum, time.Since(t1), removedCount, removedBytesSum)
	}

	stopChan := make(chan bool)

	go func() {
		for reponame, repo := range repos {
			updater(reponame, repo)
		}
		ticker := time.NewTicker(60 * time.Second)
		for {
			select {
			case <-ticker.C:
				update_free_disk_space()
				// TODO(baryluk): updater doesn't need to run if both prefetcher
				// and cache misses stats indicate there were no new writes.
				for reponame, repo := range repos {
					updater(reponame, repo)
				}
			case <-stopChan:
				ticker.Stop()
				return
			}
		}
	}()

	return stopChan
}
