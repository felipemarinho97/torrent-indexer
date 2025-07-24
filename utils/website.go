package utils

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var commonTLDs = []string{
	".com",
	".net",
	".org",
	".info",
	".biz",
	".co",
	".io",
	".xyz",
	".me",
	".tv",
	".cc",
	".us",
	".online",
	".site",
	".la",
	".se",
	".to",
}

var commonSubdomains = []string{
	"", // no prefix
	"www.",
}

var commonWebsiteSLDs = []string{
	"bludv",
	"torrentdosfilmes",
	"comando",
	"comandotorrents",
	"comandohds",
	"redetorrent",
	"torrenting",
	"baixarfilmesdubladosviatorrent",
	"hidratorrents",
	"wolverdonfilmes",
	"starckfilmes",
	"rapidotorrents",
	"sitedetorrents",
	"vamostorrent",
}

var websitePatterns = []string{
	`\[\s*ACESSE\s+%s\s*\]`,
	`\[?\s*%s\s*\]?`,
}

var regexesOnce sync.Once
var regexes []*regexp.Regexp

func getRegexes() []*regexp.Regexp {
	regexesOnce.Do(func() {
		var websites strings.Builder
		websites.WriteString("(?i)(")
		for _, prefix := range commonSubdomains {
			for _, name := range commonWebsiteSLDs {
				for _, tld := range commonTLDs {
					websites.WriteString(fmt.Sprintf("%s%s%s|", prefix, name, tld))
				}
			}
		}
		websites.WriteString(")")

		for _, pattern := range websitePatterns {
			regexes = append(regexes, regexp.MustCompile(fmt.Sprintf(pattern, websites.String())))
		}
	})
	return regexes
}

// RemoveKnownWebsites removes known website patterns from the title.
// It uses a set of common prefixes, names, and TLDs to identify and remove
// website references from the title.
// It also removes any common patterns like "[ ACESSE bludv.com ]" or
// "[ bludv.se ]" or "bludv.xyz".
func RemoveKnownWebsites(title string) string {
	regexes := getRegexes()
	for _, re := range regexes {
		title = re.ReplaceAllString(title, "")
	}
	title = strings.TrimSpace(title)
	return title
}
