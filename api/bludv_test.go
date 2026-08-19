package handler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Test_findSizeNearMagnet reproduces a BluDV multi-version post: several
// download buttons, each with its own size, followed by the magnet anchor.
// The global "Tamanho:" field lists every size at once, which used to make the
// count-based mapping fail and leave every torrent without a size. The
// per-magnet lookup must return the size sitting right next to each magnet.
func Test_findSizeNearMagnet(t *testing.T) {
	html := `<html><body><div class="content">
		<p><strong>Tamanho:</strong> 1.28 GB | 2.34 GB</p>
		<p><em>SERVIDOR PARA DOWNLOAD BluRay 720p (1.28 GB)</em></p>
		<div><a href="magnet:?xt=urn:btih:aaa"><img/></a></div>
		<p><em>SERVIDOR PARA DOWNLOAD BluRay 1080p (2.34 GB)</em></p>
		<div><a href="magnet:?xt=urn:btih:bbb"><img/></a></div>
		<p><em>SERVIDOR PARA DOWNLOAD (sem tamanho aqui)</em></p>
		<div><a href="magnet:?xt=urn:btih:ccc"><img/></a></div>
	</div></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	want := []string{"1.28 GB", "2.34 GB", ""}
	var got []string
	doc.Find(`a[href^="magnet"]`).Each(func(_ int, s *goquery.Selection) {
		got = append(got, findSizeNearMagnet(s))
	})

	if !reflect.DeepEqual(got, want) {
		t.Errorf("findSizeNearMagnet() = %v, want %v", got, want)
	}
}

// Test_findSizeNearMagnet_inlineWithBreak covers the "Vingadores Ultimato"
// layout, where the size sits in a sibling of the magnet anchor itself (same
// block, separated by a <br/>) instead of a sibling of a wrapper element.
func Test_findSizeNearMagnet_inlineWithBreak(t *testing.T) {
	html := `<html><body><div class="content"><div>` +
		`<span><em>SERVIDOR PARA DOWNLOAD BluRay 720p (2.72 GB)</em></span><br/><a href="magnet:?xt=urn:btih:aaa">Magnet-Link</a>` +
		`<span><em>SERVIDOR PARA DOWNLOAD BluRay 1080p (2.33 GB)</em></span><br/><a href="magnet:?xt=urn:btih:bbb">Magnet-Link</a>` +
		`</div></div></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	want := []string{"2.72 GB", "2.33 GB"}
	var got []string
	doc.Find(`a[href^="magnet"]`).Each(func(_ int, s *goquery.Selection) {
		got = append(got, findSizeNearMagnet(s))
	})

	if !reflect.DeepEqual(got, want) {
		t.Errorf("findSizeNearMagnet() inline layout = %v, want %v", got, want)
	}
}
