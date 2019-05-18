package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dchest/uniuri"

	"github.com/gin-gonic/gin"

	"github.com/dgrijalva/jwt-go"
)

// TwitchUser ...
type TwitchUser struct {
	ID            string    `json:"_id"`
	Bio           string    `json:"bio"`
	CreatedAt     time.Time `json:"created_at"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Logo          string    `json:"logo"`
	Name          string    `json:"name"`
	Partnered     bool      `json:"partnered"`
	Type          string    `json:"type"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type oauthResponse struct {
	AccessToken  string   `json:"access_token"`
	ExpiresIn    int      `json:"expires_in"`
	IDToken      string   `json:"id_token"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
}

var twitchHTTPClient = http.Client{}

func (ur *UnRustleLogs) getOauthToken(code string) (*oauthResponse, error) {
	oauthAPI, _ := url.Parse("https://id.twitch.tv/oauth2/token")

	query := fmt.Sprintf("client_id=%s&client_secret=%s&code=%s&grant_type=authorization_code&redirect_uri=%s",
		ur.config.Twitch.ClientID,
		ur.config.Twitch.ClientSecret,
		code,
		ur.config.Twitch.RedirectURL,
	)
	oauthAPI.RawQuery = query
	response, err := twitchHTTPClient.Post(oauthAPI.String(), "", nil)
	if err != nil {
		return nil, errors.New("failed getting oauth")
	}

	oauthBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var oauth oauthResponse
	err = json.Unmarshal(oauthBody, &oauth)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	return &oauth, nil
}

func (ur *UnRustleLogs) getUserByOAuthToken(accessToken string) (*TwitchUser, error) {
	userAPI := "https://api.twitch.tv/kraken/user"
	req, err := http.NewRequest("GET", userAPI, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", fmt.Sprintf("OAuth %s", accessToken))
	req.Header.Add("Client-ID", ur.config.Twitch.ClientID)
	req.Header.Add("Accept", "application/vnd.twitchtv.v5+json")

	response, err := twitchHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var user TwitchUser

	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// TwitchLoginHandle ...
func (ur *UnRustleLogs) TwitchLoginHandle(c *gin.Context) {
	twitchURL := "https://id.twitch.tv/oauth2/authorize"
	state := uniuri.New()
	ur.addTwitchState(state)
	s := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&force_verify=true&state=%s",
		twitchURL,
		ur.config.Twitch.ClientID,
		ur.config.Twitch.RedirectURL,
		strings.Join(ur.config.Twitch.Scopes, ","),
		state,
	)
	c.Header("Location", s)
	c.Redirect(http.StatusFound, s)
}

// TwitchLogoutHandle ...
func (ur *UnRustleLogs) TwitchLogoutHandle(c *gin.Context) {
	ur.deleteCookie(c, ur.config.Twitch.Cookie)
	c.Redirect(http.StatusFound, "/")
}

func (ur *UnRustleLogs) deleteCookie(c *gin.Context, cookie string) {
	c.SetCookie(cookie, "", -1, "/", fmt.Sprintf("%s", c.Request.Host), c.Request.URL.Scheme == "https", false)
}

// TwitchCallbackHandle ...
func (ur *UnRustleLogs) TwitchCallbackHandle(c *gin.Context) {
	state := c.Query("state")
	if !ur.hasTwitchState(state) {
		c.Redirect(http.StatusFound, "/")
		return
	}
	go ur.deleteTwitchState(state)
	code := c.Query("code")
	errorMsg := c.Query("error")
	if errorMsg != "" {
		c.String(http.StatusUnauthorized, "Authentication could not be completed because the server is misconfigured")
		return
	}
	if code == "" {
		c.String(http.StatusUnauthorized, "Authentication failed without error")
		return
	}

	oauth, err := ur.getOauthToken(code)
	if err != nil {
		log.Println(err)
		c.String(http.StatusUnauthorized, "Failed to get token from OAuth exchange code")
		return
	}

	user, err := ur.getUserByOAuthToken(oauth.AccessToken)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Twitch API failure while retrieving user")
		return
	}

	id := ur.AddTwitchUser(user)
	// Set custom claims
	claims := &jwtClaims{
		id,
		jwt.StandardClaims{
			// 1 month expire
			ExpiresAt: time.Now().Add((time.Hour * 24) * 31).Unix(),
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token and send it as response.
	t, err := token.SignedString([]byte(ur.config.Server.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed signing jwt"})
		return
	}

	c.SetCookie(ur.config.Twitch.Cookie, t, 604800, "/", fmt.Sprintf("%s", c.Request.Host), c.Request.URL.Scheme == "https", false)
	c.Redirect(http.StatusFound, "/")
}
