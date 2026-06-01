// oauth.go
package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (app *App) startOAuthFlow() {
	verifier, err := generateCodeVerifier()
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["oauth_failed_verifier"], err.Error()))
		return
	}
	challenge := generateCodeChallenge(verifier)
	state := randomString(16)

	app.oauthState = state
	app.oauthVerifier = verifier

	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&scope=openid+profile&state=%s&code_challenge=%s&code_challenge_method=S256",
		oauthAuthorizeURL, clientID, url.QueryEscape(redirectURI), state, challenge)

	// Парсим строку в *url.URL
	parsed, err := url.Parse(authURL)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["oauth_failed_parse_url"], err.Error()))
		return
	}

	if err := app.myApp.OpenURL(parsed); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["oauth_failed_open_browser"], err.Error()))
		return
	}
	app.stopNXMListener()
	go app.startCallbackServer()
}

func (app *App) startCallbackServer() {
	listener, err := net.Listen(NXMProtocol, NXMAddress)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["oauth_failed_callback_server"], err.Error()))
		return
	}
	defer listener.Close()

	var exchangeDone bool // флаг, что обмен уже выполнен

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Если уже обменяли код - не ругаемся, а вежливо отвечаем
		if exchangeDone {
			w.Write([]byte("Authorization already completed. You can close this window."))
			return
		}

		query := r.URL.Query()
		if query.Get("state") != app.oauthState {
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}

		token, err := app.exchangeCodeForToken(code, app.oauthVerifier)
		if err != nil {
			// Если ошибка из-за того, что код уже использован (invalid_grant)
			if strings.Contains(err.Error(), "invalid_grant") {
				w.Write([]byte("Authorization already completed or code expired. You can close this window."))
				return
			}
			app.appendLog(fmt.Sprintf(app.messages["oauth_exchange_failed"], err.Error()))
			http.Error(w, "Token exchange failed", http.StatusInternalServerError)
			return
		}

		exchangeDone = true // запоминаем, что обмен прошёл успешно

		app.cfg.OAuthAccessToken = token.AccessToken
		app.cfg.OAuthRefreshToken = token.RefreshToken
		app.cfg.OAuthExpiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		saveConfig(app.cfg)

		fyne.Do(func() {
			app.mainWindow.SetMainMenu(app.buildMainMenu())
			app.appendLog(app.messages["oauth_login_success"])
			// Показываем диалог, чтобы даже самый невнимательный пользователь заметил
			app.showInfoDialog(
				app.messages["oauth_success_title"],
				app.messages["oauth_success_message"],
			)
			app.startNXMListener()
		})

		w.Write([]byte("Authorization successful. You can close this window."))
		listener.Close() // закрываем сервер после успеха
	})

	http.Serve(listener, mux)
}

func (app *App) exchangeCodeForToken(code, verifier string) (*OAuthTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("code_verifier", verifier)

	// app.appendLog(fmt.Sprintf("Exchanging code: code=%s, redirect_uri=%s, client_id=%s, verifier=%s",
	//	code, redirectURI, clientID, verifier))

	req, err := http.NewRequest("POST", oauthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var token OAuthTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (app *App) refreshAccessToken() error {
	if app.cfg.OAuthRefreshToken == "" {
		return fmt.Errorf("no refresh token")
	}
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", app.cfg.OAuthRefreshToken)
	data.Set("client_id", clientID)

	req, err := http.NewRequest("POST", oauthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var token OAuthTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return err
	}
	app.cfg.OAuthAccessToken = token.AccessToken
	app.cfg.OAuthRefreshToken = token.RefreshToken
	app.cfg.OAuthExpiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	saveConfig(app.cfg)
	return nil
}

func (app *App) getAuthToken() string {
	if app.cfg.OAuthAccessToken != "" && time.Now().Before(app.cfg.OAuthExpiry) {
		return app.cfg.OAuthAccessToken
	}
	if app.cfg.OAuthAccessToken != "" && time.Now().After(app.cfg.OAuthExpiry) {
		if err := app.refreshAccessToken(); err == nil {
			return app.cfg.OAuthAccessToken
		}
		app.cfg.OAuthAccessToken = ""
		app.cfg.OAuthRefreshToken = ""
		saveConfig(app.cfg)
		fyne.Do(func() { app.mainWindow.SetMainMenu(app.buildMainMenu()) })
	}
	return app.cfg.NexusAPIKey
}

func (app *App) logoutOAuth() {
	app.cfg.OAuthAccessToken = ""
	app.cfg.OAuthRefreshToken = ""
	app.cfg.OAuthExpiry = time.Time{}
	saveConfig(app.cfg)
	fyne.Do(func() {
		app.mainWindow.SetMainMenu(app.buildMainMenu())
		app.appendLog(app.messages["oauth_logout_success"])
	})
}

// stopNXMListener закрывает слушатель nxm-ссылок, чтобы освободить порт.
func (app *App) stopNXMListener() {
	if app.nxmListener != nil {
		app.nxmListener.Close()
		app.nxmListener = nil
	}
}

// startNXMListener заново запускает слушатель nxm-ссылок после OAuth.
func (app *App) startNXMListener() {
	if app.nxmListener != nil {
		return // уже запущен
	}
	var err error
	app.nxmListener, err = net.Listen(NXMProtocol, NXMAddress)
	if err != nil {
		app.appendLog("Failed to restart nxm listener: " + err.Error())
		return
	}
	go func() {
		for {
			conn, err := app.nxmListener.Accept()
			if err != nil {
				// слушатель закрыт, выходим
				return
			}
			link, _ := bufio.NewReader(conn).ReadString('\n')
			conn.Close()
			fyne.Do(func() {
				app.handleNXMLink(strings.TrimSpace(link))
			})
		}
	}()
}
