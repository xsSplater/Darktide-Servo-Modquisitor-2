// oauth.go
package main

import (
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

    _ "embed"

    "fyne.io/fyne/v2"
    "github.com/zalando/go-keyring"
)

//go:embed assets/mechanicus.png
var mechanicusPNG []byte

const keyringService = "Servo-Modquisitor"

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

    parsed, err := url.Parse(authURL)
    if err != nil {
        app.appendLog(fmt.Sprintf(app.messages["oauth_failed_parse_url"], err.Error()))
        return
    }

    if err := app.myApp.OpenURL(parsed); err != nil {
        app.appendLog(fmt.Sprintf(app.messages["oauth_failed_open_browser"], err.Error()))
        return
    }
    go app.startCallbackServer()
}

func (app *App) startCallbackServer() {
    listener, err := net.Listen("tcp", OAuthListenAddr)
    if err != nil {
        app.appendLog(fmt.Sprintf(app.messages["oauth_failed_callback_server"], err.Error()))
        return
    }

    var exchangeDone bool
    mux := http.NewServeMux()
    server := &http.Server{Handler: mux}

    mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        if exchangeDone {
            w.Write([]byte(app.messages["authorization_completed"]))
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
            if strings.Contains(err.Error(), "invalid_grant") {
                w.Write([]byte(app.messages["authorization_completed"]))
                return
            }
            app.appendLog(fmt.Sprintf(app.messages["authorization_completed_exp"], err.Error()))
            http.Error(w, "Token exchange failed", http.StatusInternalServerError)
            return
        }

        exchangeDone = true

        // Сохраняем токены в системное хранилище
        expiry := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
        if err := app.saveOAuthTokens(token.AccessToken, token.RefreshToken, expiry); err != nil {
            app.appendLog(fmt.Sprintf("Failed to save OAuth tokens: %v", err))
        }

        fyne.Do(func() {
            app.mainWindow.SetMainMenu(app.buildMainMenu())
            app.appendLog(app.messages["oauth_login_success"])
            app.showInfoDialog(
                app.messages["oauth_success_title"],
                app.messages["oauth_success_message"],
            )
        })

        imgBase64 := base64.StdEncoding.EncodeToString(mechanicusPNG)
        imgSrc := "data:image/png;base64," + imgBase64
        successHTML := `<!DOCTYPE html>
            <html lang="en">
            <head>
                <meta charset="UTF-8">
                <title>Login Successful - Servo-Modquisitor</title>
                <style>
                    body {
                        background-color: #000000;
                        color: #c0ff1a;
                        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
                        display: flex;
                        justify-content: center;
                        align-items: center;
                        height: 100vh;
                        margin: 0;
                    }
                    .card {
                        background: #111111;
                        border: 1px solid #c0ff1a;
                        border-radius: 16px;
                        padding: 40px;
                        text-align: center;
                        max-width: 666px;
                    }
                    img.logo {
                        width: 100%;
                        max-width: 444px;
                        height: auto;
                        margin-bottom: 24px;
                        display: inline-block;
                        animation: gentlePulse 5s ease-in-out infinite;
                        will-change: transform, filter;
                        border-radius: 16px;
                    }
                    @keyframes gentlePulse {
                        0% { transform: scale(1); filter: drop-shadow(0 0 2px #c0ff1a) drop-shadow(0 0 4px #c0ff1a); }
                        50% { transform: scale(1.01); filter: drop-shadow(0 0 3px #c0ff1a) drop-shadow(0 0 6px #c0ff1a); }
                        100% { transform: scale(1); filter: drop-shadow(0 0 2px #c0ff1a) drop-shadow(0 0 4px #c0ff1a); }
                    }
                    h1 { font-size: 3.3rem; margin: 0 0 10px 0; }
                    h2 { margin: 20px 0 0 0; font-size: 2.4rem; font-weight: 400; color: #ff8866; animation: gentleClosePulse 1s ease-in-out infinite; }
                    @keyframes gentleClosePulse {
                        0%, 100% { text-shadow: 0 0 2px #ff5533, 0 0 3px #ff2222; opacity: 0.85; }
                        50% { text-shadow: 0 0 5px #ff5533, 0 0 10px #ff2222; opacity: 1; }
                    }
                    p { margin: 0 0 20px 0; font-size: 16px; color: #dddddd; }
                </style>
            </head>
            <body>
                <div class="card">
                    <img class="logo" src="` + imgSrc + `" alt="Servo-Modquisitor Logo">
                    <h1>Authorisation successful!</h1>
                    <p>You have successfully authenticated with your Nexus Mods account.</p>
                    <p>The login page on Nexus Mods can be closed as well.</p>
                    <p> </p>
                    <p> </p>
                    <h2>You may close this window now.</h2>
                </div>
            </body>
            </html>`
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write([]byte(successHTML))

        go func() {
            time.Sleep(3 * time.Second)
            server.Close()
        }()
    })

    go func() {
        time.Sleep(2 * time.Minute)
        if !exchangeDone {
            server.Close()
        }
    }()

    if err := server.Serve(listener); err != http.ErrServerClosed {
        app.appendLog("OAuth callback server error: " + err.Error())
    }
}

func (app *App) exchangeCodeForToken(code, verifier string) (*OAuthTokenResponse, error) {
    data := url.Values{}
    data.Set("grant_type", "authorization_code")
    data.Set("code", code)
    data.Set("redirect_uri", redirectURI)
    data.Set("client_id", clientID)
    data.Set("code_verifier", verifier)

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
    refreshToken, err := keyring.Get(keyringService, "refresh_token")
    if err != nil {
        return fmt.Errorf("no refresh token in keyring: %w", err)
    }
    data := url.Values{}
    data.Set("grant_type", "refresh_token")
    data.Set("refresh_token", refreshToken)
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
    expiry := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
    if err := app.saveOAuthTokens(token.AccessToken, token.RefreshToken, expiry); err != nil {
        return err
    }
    return nil
}

func (app *App) getAuthToken() string {
    accessToken, err := keyring.Get(keyringService, "access_token")
    if err != nil {
        return ""
    }
    expiryStr, err := keyring.Get(keyringService, "expiry")
    if err != nil {
        // если нет expiry, считаем, что токен недействителен
        return ""
    }
    expiry, err := time.Parse(time.RFC3339, expiryStr)
    if err != nil {
        return ""
    }
    if time.Now().Before(expiry) {
        return accessToken
    }
    // Токен истёк, пробуем обновить
    if err := app.refreshAccessToken(); err == nil {
        newToken, _ := keyring.Get(keyringService, "access_token")
        return newToken
    }
    // Если обновить не удалось, чистим хранилище
    keyring.Delete(keyringService, "access_token")
    keyring.Delete(keyringService, "refresh_token")
    keyring.Delete(keyringService, "expiry")
    fyne.Do(func() { app.mainWindow.SetMainMenu(app.buildMainMenu()) })
    return ""
}

func (app *App) logoutOAuth() {
    keyring.Delete(keyringService, "access_token")
    keyring.Delete(keyringService, "refresh_token")
    keyring.Delete(keyringService, "expiry")
    fyne.Do(func() {
        app.mainWindow.SetMainMenu(app.buildMainMenu())
        app.appendLog(app.messages["oauth_logout_success"])
    })
}

// saveOAuthTokens сохраняет токены в системное хранилище
func (app *App) saveOAuthTokens(access, refresh string, expiry time.Time) error {
    if err := keyring.Set(keyringService, "access_token", access); err != nil {
        return err
    }
    if err := keyring.Set(keyringService, "refresh_token", refresh); err != nil {
        return err
    }
    if err := keyring.Set(keyringService, "expiry", expiry.Format(time.RFC3339)); err != nil {
        return err
    }
    return nil
}
