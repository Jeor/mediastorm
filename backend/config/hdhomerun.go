package config

import (
	"errors"
	"net/url"
	"strings"
)

// HDHomeRunURL accepts a tuner IP/hostname or base URL. Only the device's
// documented endpoints are constructed; credentials and arbitrary paths are invalid.
func HDHomeRunURL(address, endpoint string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", errors.New("HDHomeRun tuner address is required")
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		(u.Path != "" && u.Path != "/" && u.Path != "/lineup.m3u") {
		return "", errors.New("HDHomeRun address must be an IP, hostname, or HTTP base URL")
	}
	u.Path = endpoint
	return u.String(), nil
}
