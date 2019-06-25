package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dchest/uniuri"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"

	"github.com/dgrijalva/jwt-go"
	"github.com/nicklaw5/helix"
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

var twitchClient *helix.Client

func (ur *UnRustleLogs) setupTwitchClient() error {
	client, err := helix.NewClient(&helix.Options{
		ClientID:     ur.config.Twitch.ClientID,
		ClientSecret: ur.config.Twitch.ClientSecret,
		RedirectURI:  ur.config.Twitch.RedirectURL,
		Scopes:       ur.config.Twitch.Scopes,
	})
	if err != nil {
		logrus.Error(err)
		return err
	}
	twitchClient = client
	return nil
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
	state := uniuri.New()
	ur.addTwitchState(state)

	url := twitchClient.GetAuthorizationURL(state, true)

	c.Header("Location", url)
	c.Redirect(http.StatusFound, url)
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
		c.String(http.StatusBadRequest, errorMsg)
		return
	}
	if code == "" {
		c.String(http.StatusUnauthorized, "Authentication failed without error")
		return
	}

	oauth, err := twitchClient.GetUserAccessToken(code)
	if err != nil {
		logrus.Error(err)
		c.String(http.StatusUnauthorized, "Failed to get token from OAuth exchange code")
		return
	}

	user, err := ur.getUserByOAuthToken(oauth.Data.AccessToken)
	if err != nil {
		logrus.Error(err)
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
		logrus.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed signing jwt"})
		return
	}

	c.SetCookie(ur.config.Twitch.Cookie, t, 604800, "/", fmt.Sprintf("%s", c.Request.Host), c.Request.URL.Scheme == "https", false)
	c.Redirect(http.StatusFound, "/")
}
