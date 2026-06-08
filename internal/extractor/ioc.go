package extractor

import (
	"regexp"
	"github.com/argus-platform/argus/internal/types"
)

var (
	reIP      = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d{2,5})?\b`)
	reDomain  = regexp.MustCompile(`\b[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z]{2,}\b`)
	reURL     = regexp.MustCompile(`https?://[^\s<>"']+`)
	reCVE     = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)
	reEmail   = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	reMD5     = regexp.MustCompile(`\b[a-fA-F0-9]{32}\b`)
	reSHA256  = regexp.MustCompile(`\b[a-fA-F0-9]{64}\b`)
)

func ExtractIOCs(content string) []types.IOC {
	var iocs []types.IOC
	seen := make(map[string]bool)

	add := func(iocType, value string) {
		if seen[value] {
			return
		}
		seen[value] = true
		iocs = append(iocs, types.IOC{Type: iocType, Value: value})
	}

	for _, m := range reIP.FindAllString(content, -1) {
		add("ip", m)
	}
	for _, m := range reDomain.FindAllString(content, -1) {
		add("domain", m)
	}
	for _, m := range reURL.FindAllString(content, -1) {
		add("url", m)
	}
	for _, m := range reCVE.FindAllString(content, -1) {
		add("cve", m)
	}
	for _, m := range reEmail.FindAllString(content, -1) {
		add("email", m)
	}
	for _, m := range reMD5.FindAllString(content, -1) {
		add("hash", m)
	}
	for _, m := range reSHA256.FindAllString(content, -1) {
		add("hash", m)
	}

	return iocs
}
