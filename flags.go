package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	listenPort = flag.Int("listen_port", 8080, "A TCP port number on which to start HTTP server to perform proxying for clients and /metrics endpoint for Prometheus monitoring")
	repoRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\.\-]+$`)
)

func splitFlag(value string) (string, string, error) {
	// parts := strings.SplitN(value, "=", 2)
	// if len(parts) != 2 {
	// 	return "", "", errors.New("Flag value invalid. Must contain =")
	// }
	// reponame := parts[0]
	// v := parts[1]

	reponame, v, good := strings.Cut(value, "=")
	if !good {
		return "", "", errors.New("Flag value invalid. Must contain =")
	}
	if len(reponame) == 0 {
		return "", "", errors.New("Flag value invalid. Repo name empty")
	}
	if !repoRegexp.Match([]byte(reponame)) {
		return "", "", errors.New("Flag value invalid. Repo name must match regexp [a-zA-Z0-9_-]+")
	}
	return reponame, v, nil
}

type UpstreamURLs map[string]string

func (i *UpstreamURLs) String() string {
	return fmt.Sprintf("%#v", *i)
}

func (i *UpstreamURLs) Set(value string) error {
	reponame, repourl, err := splitFlag(value)
	if err != nil {
		return err
	}
	_, exists := (*i)[reponame]
	if exists {
		return errors.New("Flag value invalid. Upstream URL for a repo with same name already defined")
	}
	u, err := url.Parse(repourl)
	if err != nil {
		return errors.New("Failed parsing url part of the repo definition")
	}
	if !(u.Scheme == "http" || u.Scheme == "https") {
		return errors.New("Only http and https schemas are supported")
	}
	if len(u.RawQuery) != 0 {
		return errors.New("Query part (after ? in URL) is not allowed")
	}
	if len(u.Fragment) != 0 {
		return errors.New("Fragment part (after # in URL) is not allowed")
	}
	hostname := u.Hostname()
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return errors.New("Could not resolve hostname specified in repo URL definition")
	}
	if len(addrs) == 0 {
		return errors.New("Resolving hostname specified in repo URL definition gave 0 addresses")
	}
	portStr := u.Port()
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return errors.New("Failed parsing a port in host in repo URL definition. Not a number")
		}
		if !(1 <= port && port <= 65535) {
			return errors.New("Failed parsing a port in host in repo URL definition. Out of 1-65535 range")
		}
	}
	log.Printf("Repo: %q -> %#v", reponame, u)
	(*i)[reponame] = repourl
	return nil
}

type PrefetchSpec struct {
	PrefetchType string
	PrefetchBase string
}
type PrefetchSpecs map[string]PrefetchSpec

func (i *PrefetchSpecs) String() string {
	return fmt.Sprintf("%#v", *i)
}

// This is almost identical as UpstreamURLs version,
// but with minor tweaks.
func (i *PrefetchSpecs) Set(value string) error {
	reponame, prefetchSpec, err := splitFlag(value)
	if err != nil {
		return err
	}
	_, exists := (*i)[reponame]
	if exists {
		return errors.New("Flag value invalid. Nexus prefetch URL for a repo with same name already defined")
	}

	prefetchType, prefetchDetails, good := strings.Cut(prefetchSpec, "=")
	if !good {
		return errors.New("Flag value invalid. Need to specify prefetch type")
	}
	if len(prefetchType) == 0 {
		return errors.New("Flag value invalid. Prefetch type is empty")
	}
	if !(prefetchType == "generic" || prefetchType == "nexus") {
		return errors.New("Flag value invalid. Prefetch type is unsuported")
	}

	prefetchBase := prefetchDetails

	u, err := url.Parse(prefetchBase)
	if err != nil {
		return errors.New("Failed parsing url part of the nexus prefetch definition")
	}
	if !(u.Scheme == "http" || u.Scheme == "https") {
		return errors.New("Only http and https schemas are supported")
	}
	if len(u.Fragment) != 0 {
		return errors.New("Fragment part (after # in URL) is not allowed")
	}
	hostname := u.Hostname()
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return errors.New("Could not resolve hostname specified in nexus URL definition")
	}
	if len(addrs) == 0 {
		return errors.New("Resolving hostname specified in nexus URL definition gave 0 addresses")
	}
	portStr := u.Port()
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return errors.New("Failed parsing a port in host in nexus URL definition. Not a number")
		}
		if !(1 <= port && port <= 65535) {
			return errors.New("Failed parsing a port in host in nexus URL definition. Out of 1-65535 range")
		}
	}
	log.Printf("Repo nexus url: %q -> %#v", reponame, u)
	(*i)[reponame] = PrefetchSpec{
		PrefetchType: prefetchType,
		PrefetchBase: prefetchBase,
	}
	return nil
}

type PrefetchREs map[string][]string

func (i *PrefetchREs) String() string {
	return fmt.Sprintf("%#v", *i)
}
func (i *PrefetchREs) Set(value string) error {
	reponame, re, err := splitFlag(value)
	if err != nil {
		return err
	}
	if len(re) == 0 {
		return errors.New("Flag value invalid. Empty regular expression")
	}
	// _, exists := (*i)[reponame]
	// if !exists {
	// 	(*i)[reponame] = make([]string)
	// }
	(*i)[reponame] = append((*i)[reponame], re)
	return nil
}

type GCMaxAges map[string]time.Duration

func (i *GCMaxAges) String() string {
	return fmt.Sprintf("%#v", *i)
}
func (i *GCMaxAges) Set(value string) error {
	reponame, maxAge, err := splitFlag(value)
	if err != nil {
		return err
	}
	_, exists := (*i)[reponame]
	if exists {
		return errors.New("Flag value invalid. max gc age for a repo with same name already defined")
	}
	maxAgeDuration, err := time.ParseDuration(maxAge)
	if err != nil {
		return errors.New("Flag value invalid. Invalid duration format")
	}
	(*i)[reponame] = maxAgeDuration
	return nil
}
