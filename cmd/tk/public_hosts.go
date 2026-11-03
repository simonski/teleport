package main

import (
	"errors"
	"regexp"
	"strings"
)

func parsePublicHosts(value string) ([]string, error) {
	hosts := []string{}
	seen := map[string]bool{}
	if value == "" {
		return hosts, nil
	}
	for _, host := range strings.Split(value, ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if len(host) > 253 || !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`).MatchString(host) || strings.Contains(host, "..") {
			return nil, errors.New("--public-host requires comma-separated bare hostnames")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return nil, errors.New("--public-host contains an invalid hostname label")
			}
		}
		if !seen[host] {
			hosts = append(hosts, host)
			seen[host] = true
		}
	}
	return hosts, nil
}
