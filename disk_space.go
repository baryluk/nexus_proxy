package main

import (
	"log"
	"os"

	"golang.org/x/sys/unix"
)

func update_free_disk_space() {
	var stat unix.Statfs_t
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("prefetcher: Failed to getwd to get free disk space stats. Error: %v", err)
	} else {
		unix.Statfs(wd, &stat)
		disk_cache_available_space_bytes.Set(float64(stat.Bavail * uint64(stat.Bsize)))
		disk_cache_used_space_bytes.Set(float64((stat.Blocks - stat.Bfree) * uint64(stat.Bsize)))
		disk_cache_total_space_bytes.Set(float64(stat.Blocks * uint64(stat.Bsize)))
	}
}
