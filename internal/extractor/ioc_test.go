package extractor

import (
	"testing"
)

func TestExtractIP(t *testing.T) {
	text := "C2 server at 192.168.1.1:8443 and 10.0.0.1"
	iocs := ExtractIOCs(text)

	found := false
	for _, ioc := range iocs {
		if ioc.Type == "ip" && ioc.Value == "192.168.1.1:8443" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find ip 192.168.1.1:8443")
	}
}

func TestExtractCVE(t *testing.T) {
	text := "Critical vulnerability CVE-2026-1234 disclosed"
	iocs := ExtractIOCs(text)

	found := false
	for _, ioc := range iocs {
		if ioc.Type == "cve" && ioc.Value == "CVE-2026-1234" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find CVE-2026-1234")
	}
}

func TestExtractDomain(t *testing.T) {
	text := "malware domain evil.com and https://example.com/payload"
	iocs := ExtractIOCs(text)

	domains := 0
	for _, ioc := range iocs {
		if ioc.Type == "domain" {
			domains++
		}
	}
	if domains == 0 {
		t.Error("expected to find domains")
	}
}

func TestExtractEmail(t *testing.T) {
	text := "Contact hacker@darkweb.com for exploit"
	iocs := ExtractIOCs(text)

	found := false
	for _, ioc := range iocs {
		if ioc.Type == "email" && ioc.Value == "hacker@darkweb.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find hacker@darkweb.com")
	}
}

func TestExtractHash(t *testing.T) {
	text := "file hash d41d8cd98f00b204e9800998ecf8427e and sha256 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	iocs := ExtractIOCs(text)

	hashes := 0
	for _, ioc := range iocs {
		if ioc.Type == "hash" {
			hashes++
		}
	}
	if hashes == 0 {
		t.Error("expected to find hashes")
	}
}

func TestExtractEmpty(t *testing.T) {
	text := "This is a normal text without any IOC patterns"
	iocs := ExtractIOCs(text)
	if len(iocs) != 0 {
		t.Errorf("expected 0 iocs, got %d", len(iocs))
	}
}

func TestExtractMultiple(t *testing.T) {
	text := `Threat actor APT42 (suspected state-sponsored) 
		related to CVE-2026-5678. C2 infrastructure at 45.33.32.156:443 
		and malicious domain apt42-rat.com. MD5: a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6`
	iocs := ExtractIOCs(text)

	types := make(map[string]int)
	for _, ioc := range iocs {
		types[ioc.Type]++
	}

	if types["cve"] == 0 {
		t.Error("expected CVE")
	}
	if types["ip"] == 0 {
		t.Error("expected IP")
	}
	if types["domain"] == 0 {
		t.Error("expected domain")
	}
	if types["hash"] == 0 {
		t.Error("expected hash")
	}
}
