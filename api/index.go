package handler

import (
	"fmt"
	"net/http"
	"time"
)

func HandlerIndex(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC850)
	fmt.Fprintf(w, currentTime)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		Github OAuth2 => <a href="https://github.com/xjh22222228/github-oauth2" target="_blank">https://github.com/xjh22222228/github-oauth2</a>
		`)
}
