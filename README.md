Nexus repo proxy with prefetching
---------------------------------

`nexus-proxy` a simple HTTP server acting like a mirror of a Nexus
repository. It can also be used with non-Nexus HTTP servers as a reverse
caching proxy.

It maintains a local on disk cache of files.

Periodically upstream Nexus repository is listed, and any missing files
are downloaded (in smart or configurable order, with optional include and
exclude regexps). On request the files are served from local on disk
cache, or if they are not in cache, a request to upstream Nexus
repository is made to fetch it (when fetching all redirects are handled
by the proxy). If it is there, it is served back, and saved in local on
disk cache for future serving.

Custom prefetcher can be implemented easily externally (i.e. using bash),
that injects files to the on disk cache structure.

Basic garbage collection of very old files in the cache is implemented.
Because of easy structure of the cache on disk, you can easily write your
own cron job script to clean it up.

* Configuration: Multiple upstream repositories can be configured. Each
with own URL prefix, GC policies, upstream URL, white list and prefetch
configurations. A port to listen on can also be configured. Configuration
purely via command line flags. Cache data is stored in `cache/` in
current working directory.

* Log file: A log file is a custom log, but should provide sufficient
info about request and responses. Compared to some other log formats, it
prints both at the start and end of the request, and also logs background
tasks (prefetcher and GC loops).

* Monitoring: Prometheus metric for monitoring are provided out of the
box at standard HTTP GET `/metrics` endpoint.

* Performance: about 2000MB/s on single TCP connection serving big files
from cache on unencrypted HTTP (it will eat all your CPU tho). Easily
scales to big number of clients (with more CPUs). Can do easily 20GB/s
with multiple clients. On small files I can get about 96000 requests per
second, with few ms latency. While this is not the best possible, it
should be sufficient for most use cases. Removing some logging can help
for a lot of small files, if this is what you want.

## Building

To build a binary, assuming go compiler is installed, simply run:

`go build -o nexus_proxy *.go`

(Check `build.sh` script too)

Docker container image can be produced using:

`docker build .  --tag nexus_proxy`

## Running

Simply: `./nexus_proxy --upstream_url=myrepo=http://upstream.example.com/foo/`

Then try downloading `http://localhost:8080/proxy/myrepo/index.html` for
example.

It will use `cache/` directory (and create it if needed) as a storage for
disk cache.

Note: Each `cache/REPO/` directory subtree must be on a single mount
point, so atomic `move` on files works.

Visit `http://localhost:8080/metrics` for some monitoring details.

Pass `--help` to see all options.

## Help

```
$ ./nexus_proxy --help
Usage of ./nexus_proxy:
  --upstream_url value
        (repeated) repo definitions.
        Example: --repo=mynexus=https://nexus.example.com/repository/bin42
        (default main.UpstreamURLs{})
  --prefetch value
        (repeated) prefetch repo definitions, with type and URL.
        Example: --prefetch_nexus=mynexus=nexus=https://nexus.example.com/service/rest/v1/assets?repository=maven-central
        (default main.PrefetchNexusURLs{})
  --prefetch_exclude value
        (repeated) prefetch repo exclude definitions, regular expression.
        Each repo can use multiple regexpes. If any matches, file is excluded.
        Example: --repo=mynexus=old_.* (default main.PrefetchREs{})
  --prefetch_include value
        (repeated) prefetch repo include definitions, regular expression.
        Each repo can use multiple regexpes. If any matches, file is included.
        Example: --repo=mynexus=.*.(abc|fgh)\..+ (default main.PrefetchREs{})
  --gc_max_age value
        (repeated) remove (garbage collect) files older than this time.
        Can use units, similar to golang time.ParseDuration.
        Example: --repo=mynexus=12h (default main.GCMaxAges{})
  --listen_port int
        A TCP port number on which to start HTTP server to perform proxying
        for clients and /metrics endpoint for Prometheus monitoring (default 8080)
```

During serving, first includes are processed (if non matches will stop processing),
then excludes (any matching will stop processing)

## Notes

It is recommended to have `cache/` directory on a file system with
`atime` support. This is used by garbage collection. If atime is not
updated, proxy will use creation or modification time as indicator to
when remove the file (proxy will not remove files which are still on
upstream nexus tho).

## Limitations

Currently cache is populated only at the end of the transfer. So if there
are multiple clients requesting exactly same big file at almost the same
time they will all miss, and all of them will retrieve the original from
upstream, and one of them will be put in the cache for the future
requests.

In our deployment files were less than 20MB, and most of the clients were
synchronized between each others to not do such requests concurrently.
(it could still happen from time to time tho).

If you use this proxy with significantly bigger files (1GB+), and
multiple clients might request same file at almost the same time, and you
really care about saving bandwidth to upstream Nexus, then please open a
Issue with your use case, so we can think about implementing this
("request and response gating").

Only HTTP `GET` method is supported. No support for `HEAD` or `OPTIONS`.

Only full file can be downloaded. There is no support of ranged
downloads. The data file might be still served via chunked encoding, but
it is not always, as the full file size is known at the start of serving,
even if it is a cache miss. Also if there is disconnection or other error
in a middle of the transfer, all the data is discarded from the cache
(including any temporary files). If somebody has use case to improved
this (unreliable connections, big files) please open a Issue ticket so
this could be discussed to possibly save chunks in cache and support
ranged downloads too.

The GC algorithm might not be perfect, and some stale files or
directories might still be present for very long time. If this bother
you, write a cronjob to clean them up after some time.

Current version has some UNIXisms (disk usage information, paths
handling, managing temporary files). Do not expect this program to work
on Windows or macOS.

There is no support for configuration reload. This simplifies code and
deployment.

Proxy does not care about `Vary` or `Cache-Control` HTTP headers in the
request or response. Proxy does not forward original IP of a client, or
original request headers (like `User-Agent`, `Accept-Encoding`,
`Accept-Language`, `Cookie`, `Referer`, `Origin`, etc). This is by
design. Proxy doesn't forward original `Content-Type` from upstream, or
falls back to setting always `application/octet-stream`. All other
upstream response headers are also ignored, including
`Content-Disposition`, `Cache-Control`, `Set-Cookie`, `Server`, etc.
Proxy doesn't respond with `Via` in responses, nor add `X-Client-IP`,
`X-Forwarded-For`, `Forwarded` in requests. All other headers are also
not forward from upstream, this includes `Cross-Origin-*`,
`Transfer-Encoding`, `Strict-Transport-Security`, `SourceMap`, etc.
Primary reason is that this would complicate code to support these
features, and proxy would need to keep more state for each file, which
might require usage of a proper database, which we wanted to avoid. We
are open to implementing some of these features, if there is a good
reason for them.

## Production deployment

This proxy server can be deployed as is and used directly. This works
especially well when used on internal networks, which is usually the
case.

If you want more advanced features, like TLS, authentication,
compression, ACLs, rate limits, DoS protection, standard logging, or
setting different permissions for various paths, you can place this proxy
behind some standard reverse proxy like Apache, nginx, haproxy, Squid or
Caddy, and configure it apropriately.

It is safe (but not recommended) to run multiple instances of this proxy
on the same machine with the same cache, but obviously they need to use
different ports. This can be useful when doing transparent upgrades or
changes of configuration. When doing so, it is advised to setup a initial
prefetcher delay, so two processes do not step at each other when doing a
prefetching. It will not cause any issues or data corruption, but is a
waste of network resources.

It is also safe to store cache on shared network file system or storage
(like NFS or Ceph), accessible by multiple instances of a proxy on
different machines. If many are deployed, it is advised to disable
prefetcher on most of them, and just keep one or two. Multiple proxies
then can be tried randomly by clients, setup behind single DNS name, or
behind other load balancer (i.e. `haproxy`).

Due to lack of dynamic configuration reload, or restart taking few
seconds, I recommend using a fallback to original source if the proxy
cannot be reached. In systems that only download files sporadically this
can be also avoided. Or run two proxies behind same DNS name (i.e.
multiple A or CNAME records to have simple load balancing and HA setup).

## Security

Just do not run it on public internet! You were warned.

There is no TLS or authentication.

Proxy has precautions against escaping `cache/` directory, but it does
not chroot or sandbox itself to be there. Running in a chroot or separate
namespace (container) will reduce likely hood of leaking any secret data.

## Extending to support more than Nexus

Merge Requests with support for other sources are welcome.

If you are not code savy, a simple way is to write separate script that
sends request to the proxy to prefetch all the files. This might require
some extra response headers, to not waste time processing files that are
already in a cache, or adding HEAD support.

Another option is to simply inject files directly into
`cache/REPO/final/`. This can be done for example using `rsync`,
`duplicity` or for example `wget --mirror`. It is advised to make it
atomic tho, so proxy does not see any partial files, which would result
in serving incorrect data.

## Known issues

Monitoring: Disk space used by cache files as reported by gc task, will
not count temporary files that were created using `O_TMPFILE` on Linux,
this is because these files do not show up in a directory until they are
`linkat` into final place at the end. Future fix: Track writes to these
files separately when using `O_TMPFILE`, and export separately. Note: The
used / available disk space reported for the file system as a whole will
do include these invisible / unnamed files. Proxy do export this
information, but it can also be obtained using `node_exporter`, which is
a good idea to run anyway.

On cache miss, we write to disk and to client. If the disk runs out of
space, or IO error happens, we abort both. Instead we should remove the
temporary file, but continue streaming to a client. Similarly if the
client disconnects but we are close to the end of the file and it is not
enormous file, we probably should continue downloading it to the cache
anyway.


## TODO

Parallel prefetch: Right now repos are prefetched at the same interval,
sequentially with files being prefetched sequentially. There should be
option to add paralle prefetch.

`--gc_max_age` per regexp. I.e. remove some files that do change
frequently (i.e. small metadata files), more aggressively than other
files.

Prefetch bandwidth throttling: Add option to customize global or per-repo
bandwidth limits for prefetch (but not cache miss fill).
Orderly shutdown: Shutdown, stop accepting new non-monitoring requests,
and allow existing established requests to finish, abort them if this
cannot be done in 60 seconds.

Better `/status` page.

Customizable caching and gc policies. I.e. do not cache very small files,
or cache them for very short time. Cache files with specific file
extensions longer, etc.

Route to repos based on `Host: <hostname>` request header.

Add `Age: <delta-seconds>` header to HTTP response (with 0 meaning cache
miss).

Honor a subset of `Cache-Control` from upstream server.

Remember original mime type from `Content-Type` and store in xattrs of
the file if possible.

Low priority: `HEAD` support. `ETag` support. `Last-Modified` support.

Very low priority: `Alt-Svc` support. `Digest` support.

Explore: `Content-Location` support. `If-*` support.
