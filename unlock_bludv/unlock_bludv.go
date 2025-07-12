package unlock_bludv

import (
    "fmt"
	"context"
    "regexp"
    "html"
    "net/url"

    "github.com/felipemarinho97/torrent-indexer/requester"
    "github.com/PuerkitoBio/goquery"
)

const (
	cacheKey = "lockedLink"
)

var findHrefValue = regexp.MustCompile(`href="(?<href>[\S]+)"\s`)
var hrefIndex = findHrefValue.SubexpIndex("href")

func UnlockBludvLink (req *requester.Requster, ctx context.Context, url_ string) (*string, error) {
    var err error

    redisKey := fmt.Sprintf("%s:%s", cacheKey, url_)

    result, err := req.Cache().GetString(ctx, redisKey)
	if err == nil {
		fmt.Printf("returning from locked link cache: %s\n", url_)
		return &result, nil
	}

    var doorUrl, hallUrl, cookieUrl *string

    doorUrl, err = UnlockBludvLinkFindHref(req, ctx, url_)
    if err != nil {
        return nil, err
    }
    cookieUrl = doorUrl

    hallUrl, err = UnlockBludvLinkFindHref(req, ctx, *doorUrl)
    if err == nil {
        cookieUrl = hallUrl
    }

    cookies, err := req.GetCookies(ctx, *cookieUrl)
    if err != nil {
        return nil, err
    }

    originalUrlExterna, ok := cookies["original_url_externa"]
    if !ok {
        return nil, fmt.Errorf("Cookie original_url_externa not found: %s", cookieUrl)
    }

	decodedOnce, err := url.QueryUnescape(originalUrlExterna)
	if err != nil {
		return nil, fmt.Errorf("Error to unescape once: %s", originalUrlExterna)
	}
	decodedTwice, err := url.QueryUnescape(decodedOnce)
	if err != nil {
		return nil, fmt.Errorf("Error to unescape twice: %s", decodedOnce)
	}

    unlocked := html.UnescapeString(decodedTwice)

    unlockedByte := []byte(unlocked)
    if req.Cache().Set(ctx, redisKey, unlockedByte) == nil {
        fmt.Printf("saved to cache: %s\n", url_)
    } else {
        fmt.Printf("failed to save response to cache: %v\n", err)
    }

    return &unlocked, nil
}

func UnlockBludvLinkFindHref (req *requester.Requster, ctx context.Context, url_ string) (*string, error) {
    resp, err := req.GetDocument(ctx, url_)
	if err != nil {
        return nil, err
    }
    defer resp.Close()

    doc, err := goquery.NewDocumentFromReader(resp)
	if err != nil {
        return nil, err
    }

    matches := findHrefValue.FindStringSubmatch(doc.Text())
    if matches == nil {
        return nil, fmt.Errorf("href value not found at %s", url_)
    }

    return &matches[hrefIndex], nil
}
