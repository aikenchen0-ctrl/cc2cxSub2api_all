package service

import "testing"

func TestValidateSub2APIRequestURL(t *testing.T) {
	accepted := []string{
		"https://api.cc2.cx/v1/images/generations",
		"https://api.cc2.cx/v1/chat/completions",
		"http://sub2api:8080/v1/images/edits",
		"https://api.cc2.cx/v1beta/models/gpt-image-2:generateContent",
	}
	for _, raw := range accepted {
		if err := ValidateSub2APIRequestURL(raw); err != nil {
			t.Fatalf("rejected %q: %v", raw, err)
		}
	}

	rejected := []string{
		"https://api.cc2.cx/api/v1/v1/images/generations",
		"https://api.cc2.cx/api/v1/images/generations",
		"https://api.cc2.cx/v1/v1/images/generations",
		"https://api.cc2.cx/images/generations",
		"https://api.cc2.cx/",
		"https://api.cc2.cx",
		"api.cc2.cx/v1/images/generations",
		"ftp://api.cc2.cx/v1/images/generations",
		"",
	}
	for _, raw := range rejected {
		if err := ValidateSub2APIRequestURL(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
