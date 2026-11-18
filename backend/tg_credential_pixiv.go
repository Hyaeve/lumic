package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func checkTGCredentialPixiv(ctx context.Context, credentials PixivCredentials, proxyURL string) tgQRCredentialStatus {
	if ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	credentials.UserID = strings.TrimSpace(credentials.UserID)
	if strings.TrimSpace(credentials.Cookie) == "" || strings.TrimSpace(credentials.UserAgent) == "" || !tgPixivViewerIDValid(credentials.UserID) {
		return tgQRCredentialInvalid
	}
	// A public user profile can be read anonymously; check the site's own viewer.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.pixiv.net/", nil)
	if err != nil {
		return tgQRCredentialUnknown
	}
	setPixivBrowserHeaders(request, credentials)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return tgQRCredentialUnknown
	}
	defer client.CloseIdleConnections()
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.Scheme != "https" || next.URL.Host != "www.pixiv.net" || next.URL.User != nil || tgPixivLoginRedirect(next.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return tgQRCredentialUnknown
	}
	defer response.Body.Close()
	if ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	if response.StatusCode == http.StatusUnauthorized {
		return tgQRCredentialInvalid
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		if target, err := response.Location(); err == nil && tgPixivLoginRedirect(target) {
			return tgQRCredentialInvalid
		}
		return tgQRCredentialUnknown
	}
	if response.StatusCode != http.StatusOK {
		return tgQRCredentialUnknown
	}
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit || ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	return tgPixivPageViewerStatus(data, credentials.UserID)
}

func tgPixivLoginRedirect(target *url.URL) bool {
	if target.Scheme != "https" || target.User != nil {
		return false
	}
	return target.Host == "accounts.pixiv.net" && (target.Path == "/login" || target.Path == "/login/") ||
		target.Host == "www.pixiv.net" && target.Path == "/login.php"
}

func tgPixivViewerIDValid(id string) bool {
	number, err := strconv.ParseUint(id, 10, 64)
	return err == nil && number > 0 && strconv.FormatUint(number, 10) == id
}

func tgPixivViewerIdentity(raw json.RawMessage, expectedID string) tgQRCredentialStatus {
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		var number json.Number
		if json.Unmarshal(raw, &number) != nil {
			return tgQRCredentialUnknown
		}
		id = number.String()
	}
	if !tgPixivViewerIDValid(id) {
		return tgQRCredentialUnknown
	}
	if id != expectedID {
		return tgQRCredentialInvalid
	}
	return tgQRCredentialValid
}

func tgPixivPageViewerStatus(data []byte, expectedID string) tgQRCredentialStatus {
	tokens := html.NewTokenizer(bytes.NewReader(data))
	var next, legacy []byte
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			if tokens.Err() != io.EOF {
				return tgQRCredentialUnknown
			}
			return tgPixivViewerMetadataStatus(next, legacy, expectedID)
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			id, content := "", ""
			for _, attr := range token.Attr {
				switch attr.Key {
				case "id":
					id = attr.Val
				case "content":
					content = attr.Val
				}
			}
			if token.Data == "meta" && id == "meta-global-data" {
				if legacy != nil {
					return tgQRCredentialUnknown
				}
				legacy = []byte(content)
			}
			if token.Data == "script" && id == "__NEXT_DATA__" {
				if next != nil || tokens.Next() != html.TextToken {
					return tgQRCredentialUnknown
				}
				next = append([]byte{}, tokens.Text()...)
			}
		}
	}
}

func tgPixivViewerMetadataStatus(next, legacy []byte, expectedID string) tgQRCredentialStatus {
	status := tgQRCredentialUnknown
	if next != nil {
		var page struct {
			Props struct {
				PageProps struct {
					IsLoggedIn *bool `json:"isLoggedIn"`
					GAUserData struct {
						Login  *bool           `json:"login"`
						UserID json.RawMessage `json:"userId"`
					} `json:"gaUserData"`
				} `json:"pageProps"`
			} `json:"props"`
		}
		if json.Unmarshal(next, &page) != nil || page.Props.PageProps.IsLoggedIn == nil {
			return tgQRCredentialUnknown
		}
		viewer := page.Props.PageProps
		if !*viewer.IsLoggedIn {
			if viewer.GAUserData.Login != nil && *viewer.GAUserData.Login {
				return tgQRCredentialUnknown
			}
			status = tgQRCredentialInvalid
		} else if viewer.GAUserData.Login != nil && *viewer.GAUserData.Login {
			status = tgPixivViewerIdentity(viewer.GAUserData.UserID, expectedID)
		}
	}
	if legacy != nil {
		var global struct {
			UserData json.RawMessage `json:"userData"`
		}
		if json.Unmarshal(legacy, &global) != nil || global.UserData == nil {
			return tgQRCredentialUnknown
		}
		old := tgQRCredentialInvalid
		if !bytes.Equal(bytes.TrimSpace(global.UserData), []byte("null")) {
			var viewer struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(global.UserData, &viewer) != nil {
				return tgQRCredentialUnknown
			}
			old = tgPixivViewerIdentity(viewer.ID, expectedID)
		}
		if next != nil && status != old {
			return tgQRCredentialUnknown
		}
		status = old
	}
	return status
}
