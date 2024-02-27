package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const bufferSize = 65536

type Repo struct {
	upstreamURLBase        string
	gcMaxAge               time.Duration
	prefetchType           string
	prefetchBase           string
	prefetchIncludeRegexps []*regexp.Regexp
	prefetchExcludeRegexps []*regexp.Regexp
}

func main() {
	ret := main2()
	if ret != 0 {
		os.Exit(ret)
	}
}

func main2() int {
	upstreamURLs := make(UpstreamURLs)
	prefetchSpecs := make(PrefetchSpecs)
	prefetchIncludeREs := make(PrefetchREs)
	prefetchExcludeREs := make(PrefetchREs)
	gcMaxAges := make(GCMaxAges)
	flag.Var(&upstreamURLs, "upstream_url", "(repeated) repo definitions. Example: --upstream_url=mynexus=https://nexus.example.com/repository/bin42")
	flag.Var(&prefetchSpecs, "prefetch", "(repeated) prefetch repo definitions, in form of reponame=prefetchType=prefetchDetails. Example: --prefetch=mynexus=nexus=https://nexus.example.com/service/rest/v1/assets?repository=maven-central")
	flag.Var(&prefetchIncludeREs, "prefetch_include", "(repeated) prefetch repo include definitions, regular expression. Each repo can use multiple regexpes. If any matches, file is included. Example: --prefetch_include=mynexus=.*.(abc|fgh)\\..+")
	flag.Var(&prefetchExcludeREs, "prefetch_exclude", "(repeated) prefetch repo exclude definitions, regular expression. Each repo can use multiple regexpes. If any matches, file is excluded. Example: --prefetch_exclude=mynexus=old_.*")
	flag.Var(&gcMaxAges, "gc_max_age", "(repeated) remove (garbage collect) files older than this time. Can use units, similar to golang time.ParseDuration. Example: --gc_max_age=mynexus=12h")
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return 1
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)

	if len(upstreamURLs) == 0 {
		log.Printf("Need to specify some repos using --upstream_url argument")
		return 1
	}

	log.Printf("upstreamURLs: %#v", upstreamURLs)
	log.Printf("prefetchIncludeREs: %#v", prefetchIncludeREs)
	log.Printf("prefetchExcludeREs: %#v", prefetchExcludeREs)

	repos := make(map[string]*Repo)
	for reponame, upstreamURLBase := range upstreamURLs {
		repos[reponame] = &Repo{
			upstreamURLBase: upstreamURLBase,
		}
	}
	for reponame, prefetchSpec := range prefetchSpecs {
		repo, exists := repos[reponame]
		if !exists {
			log.Fatalf("Repo name %q referenced in --prefetch is not defined by any --upstream_url argument", reponame)
		}
		repo.prefetchType = prefetchSpec.PrefetchType
		repo.prefetchBase = prefetchSpec.PrefetchBase
	}
	for reponame, includeREs := range prefetchIncludeREs {
		repo, exists := repos[reponame]
		if !exists {
			log.Fatalf("Repo name %q referenced in --prefetch_include is not defined by any --upstream_url argument", reponame)
		}
		if len(repo.prefetchBase) == 0 {
			log.Fatalf("Repo name %q referenced in --prefetch_include has no --prefetch argument", reponame)
		}
		for _, includeRE := range includeREs {
			repo.prefetchIncludeRegexps = append(repo.prefetchIncludeRegexps, regexp.MustCompile(includeRE))
		}
	}
	for reponame, excludeREs := range prefetchExcludeREs {
		repo, exists := repos[reponame]
		if !exists {
			log.Fatalf("Repo name %q referenced in --prefetch_exclude is not defined by any --upstream_url argument", reponame)
		}
		if len(repo.prefetchBase) == 0 {
			log.Fatalf("Repo name %q referenced in --prefetch_exclude has no --prefetch argument", reponame)
		}
		for _, excludeRE := range excludeREs {
			repo.prefetchExcludeRegexps = append(repo.prefetchExcludeRegexps, regexp.MustCompile(excludeRE))
		}
	}
	for reponame, maxAge := range gcMaxAges {
		repo, exists := repos[reponame]
		if !exists {
			log.Fatalf("Repo name %q referenced in --gc_max_age is not defined by any --upstream_url argument", reponame)
		}
		repo.gcMaxAge = maxAge
	}

	for reponame, repo := range repos {
		log.Printf("repo %q: %#v", reponame, repo)
	}

	err := os.MkdirAll("cache", os.ModePerm)
	if err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}

	for reponame := range repos {
		err = os.MkdirAll("cache/"+reponame, os.ModePerm)
		if err != nil && !os.IsExist(err) {
			log.Fatal(err)
		}
		err = os.MkdirAll("cache/"+reponame+"/temp", os.ModePerm)
		if err != nil && !os.IsExist(err) {
			log.Fatal(err)
		}
		err = os.MkdirAll("cache/"+reponame+"/final", os.ModePerm)
		if err != nil && !os.IsExist(err) {
			log.Fatal(err)
		}
	}

	update_free_disk_space()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/proxy/", proxyHandler(repos))

	prefetchStopChan := startPrefetchLoop(repos)
	gcStopChan := startGCLoop(repos)

	listenSpec := ":" + strconv.Itoa(*listenPort)
	log.Printf("Starting listening on %q\n", listenSpec)
	log.Fatal(http.ListenAndServe(":8080", nil))

	prefetchStopChan <- true
	gcStopChan <- true

	return 0
}
