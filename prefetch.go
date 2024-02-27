package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// https://help.sonatype.com/repomanager3/integrations/rest-and-integration-api/assets-api#AssetsAPI-ListAssets
// 'http://localhost:8081/service/rest/v1/assets?repository=maven-central'
type NexusItem struct {
	DownloadUrl string            `json:"downloadUrl"`
	Path        string            `json:"path"`
	Id          string            `json:"id"`
	Repository  string            `json:"repository"`
	Format      string            `json:"format"`
	Checksums   map[string]string `json:"checksum"`
}

type NexusAssetsResponse struct {
	Items             []NexusItem `json:"items"`
	ContinuationToken string      `json:"continuationToken"`
}

// https://help.sonatype.com/repomanager3/integrations/rest-and-integration-api/assets-api#AssetsAPI-GetAsset
// 'http://localhost:8081/service/rest/v1/assets/bWF2ZW4tY2VudHJhbDozZjVjYWUwMTc2MDIzM2I2MjRiOTEwMmMwMmNiYmU4YQ'
// type NexusAsset struct {
// }

func startPrefetchLoop(repos map[string]*Repo) chan bool {
	prefetchInterval := 60 * time.Second

	matcher := func(repo *Repo, filename string) bool {
		// log.Printf("prefetcher: Checking %q", filename)
		if repo.prefetchIncludeRegexps != nil {
			include := false
			for _, includeRegexp := range repo.prefetchIncludeRegexps {
				if includeRegexp.Match([]byte(filename)) {
					include = true
					break
				}
			}
			if !include {
				// log.Printf("prefetcher: Not including %q", filename)
				prefetch_ignore_count.Inc()
				return false
			}
		}

		if repo.prefetchExcludeRegexps != nil {
			for _, excludeRegexp := range repo.prefetchExcludeRegexps {
				if excludeRegexp.Match([]byte(filename)) {
					// log.Printf("prefetcher: Excluding %q", filename)
					prefetch_ignore_count.Inc()
					return false
				}
			}
		}

		return true
	}

	process := func(reponame string, repo *Repo, item NexusItem) error {
		filename := item.Path

		if !matcher(repo, filename) {
			return nil
		}

		// log.Printf("prefetcher: Processing %#v", item)

		cacheFilename := "cache/" + reponame + "/final/" + filename
		if _, err := os.Stat(cacheFilename); !errors.Is(err, os.ErrNotExist) {
			prefetch_skip_count.Inc()
			return nil
		}

		log.Printf("prefetcher: Prefetching missing %#v", item)

		resp, err := http.Get(repo.upstreamURLBase + filename)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		cacheTemp, err := NewTempFile("cache/"+reponame+"/temp/", filename, cacheFilename)
		if err != nil {
			return err
		}
		bytesCopiedCount, err := io.Copy(cacheTemp.File(), resp.Body)
		prefetch_download_bytes.Add(float64(bytesCopiedCount))
		if err != nil {
			cacheTemp.Cleanup()
			return err
		} else {
			err = cacheTemp.Finalize()
			prefetch_download_count.Inc()
		}
		update_free_disk_space()
		return err
	}

	updater := func(reponame string, repo *Repo) {
		url := repo.prefetchBase
		if len(url) == 0 {
			log.Printf("prefetcher: Skipping update loop for repo %q", reponame)
			return
		}
		if len(repo.prefetchType) == 0 {
			log.Printf("prefetcher: Skipping update loop for repo %q", reponame)
			return
		}

		log.Printf("prefetcher: Update loop started")
		t1 := time.Now()

		update_free_disk_space()

		timer := prometheus.NewTimer(prefetch_loop_time)
		defer timer.ObserveDuration()
		prefetch_in_progress.Set(1)
		defer func() {
			last_prefetch_time.Set(time.Since(t1).Seconds())
			prefetch_in_progress.Set(0)
		}()

		nexusClient := http.Client{
			Timeout: 30 * time.Second,
		}

		// https://help.sonatype.com/repomanager3/integrations/rest-and-integration-api/assets-api#AssetsAPI-ListAssets
		// https://github.com/sonatype/nexus-public/pull/51

		// Faster listing using:
		// https://nexus.example.com/service/rest/repository/browse/binaries/
		// Inside you will find:
		//            <td><a href="https://nexus.example.com/repository/binaries/foobar.49b45dc0fexyz.2208010203">foobar.49b45dc0fexyz.2208010203</a></td>
		// The list might be truncated, but usually isn't.
		// There is also "?filter=..." parameters, but I do not know how it works.

		/*
		   Apache output

		   <tr><td valign="top"><img src="/icons/back.gif" alt="[PARENTDIR]"></td><td><a href="/">Parent Directory</a></td><td>&nbsp;</td><td align="right">  - </td></tr>
		   <tr><td valign="top"><img src="/icons/hand.right.gif" alt="[   ]"></td><td><a href="README">README</a></td><td align="right">2022-07-09 08:24  </td><td align="right">1.3K</td></tr>
		   <tr><td valign="top"><img src="/icons/folder.gif" alt="[DIR]"></td><td><a href="dists/">dists/</a></td><td align="right">2022-07-09 08:26  </td><td align="right">  - </td></tr>

		*/

		/* apache also
		   <tr><td valign="top"><img src="/icons/back.gif" alt="[PARENTDIR]"></td><td><a href="/">Parent Directory</a></td><td>&nbsp;</td><td align="right">  - </td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/text.gif" alt="[TXT]"></td><td><a href="LATEST.txt">LATEST.txt</a></td><td align="right">2022-07-13 23:41  </td><td align="right"> 34 </td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/unknown.gif" alt="[   ]"></td><td><a href="PACKAGES.list">PACKAGES.list</a></td><td align="right">2022-07-13 22:06  </td><td align="right"> 57K</td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/text.gif" alt="[TXT]"></td><td><a href="PACKAGES.md">PACKAGES.md</a></td><td align="right">2021-10-23 17:36  </td><td align="right"> 69K</td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/text.gif" alt="[TXT]"></td><td><a href="README.html">README.html</a></td><td align="right">2022-07-13 23:41  </td><td align="right">  0 </td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/text.gif" alt="[TXT]"></td><td><a href="README.md">README.md</a></td><td align="right">2022-07-15 20:50  </td><td align="right"> 54K</td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/text.gif" alt="[TXT]"></td><td><a href="SHA256SUMS.txt">SHA256SUMS.txt</a></td><td align="right">2022-07-13 23:31  </td><td align="right">2.5K</td><td>&nbsp;</td></tr>
		   <tr><td valign="top"><img src="/icons/folder.gif" alt="[DIR]"></td><td><a href="Videos/">Videos/</a></td><td align="right">2022-06-22 02:24  </td><td align="right">  - </td><td>&nbsp;</td></tr>
		*/
		/* apache , cdimage.debian.org
		   <table id="indexlist">
		    <tr class="indexhead"><th class="indexcolicon"><img src="/icons2/blank.png" alt="[ICO]"></th><th class="indexcolname"><a href="?C=N;O=D">Name</a></th><th class="indexcollastmod"><a href="?C=M;O=A">Last modified</a></th><th class="indexcolsize"><a href="?C=S;O=A">Size</a></th></tr>
		    <tr class="indexbreakrow"><th colspan="4"><hr></th></tr>
		    <tr class="even"><td class="indexcolicon"><a href="/cdimage/ports/"><img src="/icons2/go-previous.png" alt="[PARENTDIR]"></a></td><td class="indexcolname"><a href="/cdimage/ports/">Parent Directory</a></td><td class="indexcollastmod">&nbsp;</td><td class="indexcolsize">  - </td></tr>
		    <tr class="odd"><td class="indexcolicon"><a href="2019-01-25/"><img src="/icons2/folder.png" alt="[DIR]"></a></td><td class="indexcolname"><a href="2019-01-25/">2019-01-25/</a></td><td class="indexcollastmod">2019-01-25 00:14  </td><td class="indexcolsize">  - </td></tr>
		*/
		/* nginx (mirror.init7.net)
		   table id="list"><thead><tr><th style="width:55%"><a href="?C=N&amp;O=A">File Name</a>&nbsp;<a href="?C=N&amp;O=D">&nbsp;&darr;&nbsp;</a></th><th style="width:20%"><a href="?C=S&amp;O=A">File Size</a>&nbsp;<a href="?C=S&amp;O=D">&nbsp;&darr;&nbsp;</a></th><th style="width:25%"><a href="?C=M&amp;O=A">Date</a>&nbsp;<a href="?C=M&amp;O=D">&nbsp;&darr;&nbsp;</a></th></tr></thead>
		   <tbody><tr><td class="link"><a href="../">Parent directory/</a></td><td class="size">-</td><td class="date">-</td></tr>
		   <tr><td class="link"><a href="edge/" title="edge">edge/</a></td><td class="size">-</td><td class="date">2015-09-30 09:58:27 </td></tr>
		   <tr><td class="link"><a href="latest-stable/" title="latest-stable">latest-stable/</a></td><td class="size">-</td><td class="date">2022-05-16 21:04:02 </td></tr>
		   <tr><td class="link"><a href="v3.0/" title="v3.0">v3.0/</a></td><td class="size">-</td><td class="date">2014-05- 8 00:52:55 </td></tr>
		*/
		/* nginx fancy index https://github.com/aperezdc/ngx-fancyindex/blob/master/template.html
		   <table id="list">
		   			<thead>
		   				<tr>
		   					<th colspan="2"><a href="?C=N&amp;O=A">File Name</a>&nbsp;<a href="?C=N&amp;O=D">&nbsp;&darr;&nbsp;</a></th>
		   					<th><a href="?C=S&amp;O=A">File Size</a>&nbsp;<a href="?C=S&amp;O=D">&nbsp;&darr;&nbsp;</a></th>
		   					<th><a href="?C=M&amp;O=A">Date</a>&nbsp;<a href="?C=M&amp;O=D">&nbsp;&darr;&nbsp;</a></th>
		   				</tr>
		   			</thead>

		   			<tbody>
		   <!-- var t_parentdir_entry -->
		   				<tr>
		   					<td colspan="2" class="link"><a href="../?C=N&amp;O=A">Parent directory/</a></td>
		   					<td class="size">-</td>
		   					<td class="date">-</td>
		   				</tr>

		   <!-- var NONE -->
		   				<tr>
		   					<td colspan="2">test file 1</td>
		   					<td>123kB</td>
		   					<td>date</td>
		   				</tr>
		*/

		// TODO(baryluk): Rate limits after some amount of requests.
		// Both request number and bytes transfered.
		// This way prefetch can be used fairly on public and 3rd party
		// endpoints, without overloading them.
		totalFetches := 0

		recursionLimit := 10000
		discoveredLinksCount := 0

		skippedFiles := 0
		skippedDirs := 0

		if repo.prefetchType == "generic" {
			var recursor func(url string, depth int) error
			recursor = func(url string, depth int) error {
				totalFetches++
				if totalFetches > recursionLimit {
					return errors.New("Not recursing futher, as already reached 1000 requests")
				}

				time.Sleep(10 * time.Millisecond)
				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					prefetch_list_error_count.Inc()
					return err
				}
				req.Header.Set("User-Agent", "nexus-proxy")
				resp, err := nexusClient.Do(req)
				if err != nil {
					prefetch_list_error_count.Inc()
					log.Printf("prefetcher: Cannot make a request. Error: %v", err)
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					prefetch_list_error_count.Inc()
					log.Printf("prefetcher: Error response. Status: %d", resp.StatusCode)
					return nil // TODO
				}

				scanner := bufio.NewScanner(resp.Body)

				// TODO(baryluk): Before recursing we should consume full body,
				// otherwise it can take minutes or hours before we go back
				// from recursion, by which time the connection timed out,
				// and we cannot consume rest of links.
				// (As long as it is reasonably small, <10MB).

				re := regexp.MustCompile(`<[Aa] +(?:href|HREF)="([^"]+)"( |>)`)
				reSchema := regexp.MustCompile(`^[a-zA-Z]+://`)
				for scanner.Scan() {
					line := scanner.Text()
					m := re.FindSubmatch([]byte(line))
					if m != nil {
						href := string(m[1])
						if reSchema.Match([]byte(href)) {
							continue
						}
						if strings.HasPrefix(href, "../") || strings.HasPrefix(href, "/") || strings.HasPrefix(href, "?") {
							continue
						}
						if strings.Contains(href, "&") {
							continue
						}
						if !matcher(repo, href) {
							if strings.HasSuffix(href, "/") {
								skippedDirs++
								// log.Printf("prefetcher: Skipping %d dir %q", skippedDirs, url + href)
							} else {
								skippedFiles++
								// log.Printf("prefetcher: Skipping %d file %q", skippedFiles, url + href)
							}
							continue
						}

						discoveredLinksCount++
						log.Printf("prefetcher: Matched %d %q", discoveredLinksCount, url+href)
						if strings.HasSuffix(href, "/") {
							if totalFetches < recursionLimit {
								err := recursor(url+href, depth+1)
								if err != nil {
									log.Printf("prefetcher: Error recursing: %v", err)
								}
							}
						}
					}
					if false {
						item := NexusItem{}
						err = process(reponame, repo, item)
						if err != nil {
							log.Printf("prefetcher: Failed to process item. Error: %v", err)
							prefetch_download_error_count.Inc()
						}
					}
				}
				if err = scanner.Err(); err != nil {
					log.Printf("prefetcher: Read error %v:", err)
				}
				return err
			}

			recursor(url, 0)
		} else if repo.prefetchType == "nexus" {
			continuationToken := ""
			for {
				var urlWithContinuation string
				if len(continuationToken) > 0 {
					urlWithContinuation = url + "&continuationToken=" + continuationToken
				} else {
					urlWithContinuation = url
				}
				prefetch_list_request_count.Inc()
				req, err := http.NewRequest(http.MethodGet, urlWithContinuation, nil)
				if err != nil {
					prefetch_list_error_count.Inc()
					break
				}
				req.Header.Set("User-Agent", "nexus-proxy")
				resp, err := nexusClient.Do(req)
				if err != nil {
					prefetch_list_error_count.Inc()
					log.Printf("prefetcher: Cannot make a request. Error: %v", err)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					prefetch_list_error_count.Inc()
					log.Printf("prefetcher: Error response. Status: %d", resp.StatusCode)
					return
				}
				response := NexusAssetsResponse{}
				jsonDecoder := json.NewDecoder(resp.Body)
				err = jsonDecoder.Decode(&response)
				if err != nil {
					log.Printf("prefetcher: Failed to JSON Assets response JSON. Error: %v", err)
					prefetch_list_error_count.Inc()
					return
				}
				if jsonDecoder.More() {
					log.Printf("prefetcher: Warning: Found more tokens after first JSON object decoded in Nexus response. Ignoring")
				}
				if response.Items != nil {
					for _, item := range response.Items {
						err = process(reponame, repo, item)
						if err != nil {
							log.Printf("prefetcher: Failed to process item. Error: %v", err)
							prefetch_download_error_count.Inc()
						}
					}
				}
				if len(response.ContinuationToken) > 0 {
					continuationToken = response.ContinuationToken
				} else {
					break
				}
			}
		} else {
			log.Printf("prefetcher: Unknown prefetchType in repo %s : %#v", reponame, repo)
		}
		log.Printf("prefetcher: Update loop finished in %s", time.Since(t1))

		update_free_disk_space()
	}

	stopChan := make(chan bool)

	go func() {
		log.Printf("prefetcher: Initial prefetch started")
		for reponame, repo := range repos {
			updater(reponame, repo)
		}
		log.Printf("prefetcher: Initial prefetch finished")
		log.Printf("prefetcher: Starting main loop")
		ticker := time.NewTicker(prefetchInterval)
		for {
			select {
			case <-ticker.C:
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
