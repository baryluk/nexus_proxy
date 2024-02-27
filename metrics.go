package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics
var (
	requests_in_progress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_requests_in_progress",
		Help: "Number of requests being served right now",
	})
	hit_requests_in_progress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_hit_requests_in_progress",
		Help: "Number of hit requests being served right now",
	})
	miss_requests_in_progress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_miss_requests_in_progress",
		Help: "Number of miss requests being served right now",
	})

	hit_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_hit_count",
		Help: "The total number of cache hits",
	})
	hit_bytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_hit_bytes",
		Help: "The total number bytes served from local cache (including ending in error)",
	})
	miss_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_miss_count",
		Help: "The total number of cache misses",
	})
	miss_bytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_miss_bytes",
		Help: "The total number bytes served from upstream (and saved in local cache, even if ended in error)",
	})
	error_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_error_count",
		Help: "The total number of errors - including client bad requests, bad repo, disconnection, upstream 404, etc.",
	})
	upstream_error_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_upstream_error_count",
		Help: "The total number of upstream errors received - connection issues or non-200 error codes",
	})
	prefetch_ignore_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_ignore_count",
		Help: "Number of items listed in upstream Nexus, but excluded due to not matching regexp",
	})
	prefetch_skip_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_skip_count",
		Help: "Number of items listed in upstream Nexus, but skipped because it is already in local cache",
	})
	prefetch_download_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_download_count",
		Help: "Number of items from upstream Nexus, prefetched",
	})
	prefetch_download_bytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_download_bytes",
		Help: "Number of bytes (actual data, not including HTTP protocol overheads) from upstream Nexus, prefetched. Includes failed downloades",
	})
	prefetch_download_error_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_download_error_count",
		Help: "Number of download try errors",
	})
	prefetch_list_request_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_list_request_count",
		Help: "Number of repo list API requests made (no matter the error status)",
	})
	prefetch_list_error_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_prefetch_list_error_count",
		Help: "Number of repo list API errors",
	})
	prefetch_in_progress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_prefetch_in_progress",
		Help: "Is prefetch in progress?",
	})
	prefetch_loop_time = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "nexus_proxy_prefetch_loop_time_seconds",
		Help: "Total time of prefetch loop (including listing and downloading) and total count of them",
	})
	last_prefetch_time = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_prefetch_last_loop_time_seconds",
		Help: "How much time the last prefetch took",
	})
	// last_successfull_prefetch_timestamp
	disk_cache_size_bytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_disk_cache_size_bytes",
		Help: "Size of on disk cache (sum of all files sizes)",
	})
	disk_cache_available_space_bytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_disk_cache_available_space_bytes",
		Help: "Amount of available space in cache directory as reported by OS",
	})
	disk_cache_used_space_bytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_disk_cache_used_space_bytes",
		Help: "Amount of used space in cache directory as reported by OS",
	})
	disk_cache_total_space_bytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_disk_cache_total_space_bytes",
		Help: "Amount of total space in cache directory as reported by OS",
	})
	gc_error_count = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_proxy_gc_error_count",
		Help: "Total number of errors encountered during GC, i.e. permissions errors, remove file errors",
	})
	gc_in_progress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_gc_in_progress",
		Help: "Is garbage collection in progress?",
	})
	gc_loop_time = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "nexus_proxy_gc_loop_time_seconds",
		Help: "Total time of gc loop (including listing and removing) and total count of them",
	})
	last_gc_time = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_gc_last_loop_time_seconds",
		Help: "How much time the last gc loop took",
	})
	// last_successfull_gc_timestamp
	gc_final_size = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_gc_final_files_size_bytes",
		Help: "How many file bytes remaining in all directories",
	})
	gc_final_count = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_proxy_gc_final_files_count",
		Help: "How many file  remaining in all directories",
	})
)

/* TODO

time of last gc start
time of last gc end
time of start of current gc
time of last successfull gc

same for prefetch

statistics of previous gc and prefetch
statistics of current gc and prefetch

*/
