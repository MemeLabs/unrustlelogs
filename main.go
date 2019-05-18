package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// UnRustleLogs ...
type UnRustleLogs struct {
	config *Config
	db     *gorm.DB

	dggStates     map[string]*state
	dggStateMutex sync.RWMutex

	twitchStates     map[string]struct{}
	twitchStateMutex sync.RWMutex
}

type state struct {
	service  string
	verifier string
	time     time.Time
}

const (
	// TWITCHSERVICE ...
	TWITCHSERVICE = "twitch"
	// DESTINYGGSERVICE ...
	DESTINYGGSERVICE = "destinygg"
)

// jwtCustomClaims are custom claims extending default ones.
type jwtClaims struct {
	ID string `json:"id"`
	jwt.StandardClaims
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	rustle := NewUnRustleLogs()
	rustle.LoadConfig("config.toml")

	rustle.NewDatabase()
	err := rustle.setupTwitchClient()
	if err != nil {
		logrus.Fatal(err)
	}

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	router.GET("/", rustle.indexHandler)
	router.GET("/verify", rustle.verifyHandler)
	router.GET("/robots.txt", func(c *gin.Context) {
		c.String(200, "User-agent: *\nDisallow: /")
	})

	twitch := router.Group("/twitch")
	{
		twitch.GET("/login", rustle.TwitchLoginHandle)
		twitch.GET("/logout", rustle.TwitchLogoutHandle)
		twitch.GET("/callback", rustle.TwitchCallbackHandle)
	}

	dgg := router.Group("/dgg")
	{
		dgg.GET("/login", rustle.DestinyggLoginHandle)
		dgg.GET("/logout", rustle.DestinyggLogoutHandle)
		dgg.GET("/callback", rustle.DestinyggCallbackHandle)
	}

	router.Static("/assets", "./assets")

	srv := &http.Server{
		Handler: router,
		Addr:    rustle.config.Server.Address,
		// Good practice: enforce timeouts for servers you create!
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logrus.Infof("starting server adress: %q", rustle.config.Server.Address)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Error(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	if err := srv.Shutdown(ctx); err != nil {
		logrus.Fatal("Server Shutdown:", err)
	}
	logrus.Info("Server exiting")
}

// NewUnRustleLogs ...
func NewUnRustleLogs() *UnRustleLogs {
	return &UnRustleLogs{
		dggStates:    make(map[string]*state),
		twitchStates: make(map[string]struct{}),
	}
}

// Payload ...
type Payload struct {
	Twitch struct {
		ID       string
		Name     string
		Email    string
		LoggedIn bool
	}
	Destinygg struct {
		ID       string
		Name     string
		LoggedIn bool
	}
}

func (ur *UnRustleLogs) indexHandler(c *gin.Context) {
	payload := Payload{}
	twitch, ok := ur.getUserFromJWT(c, ur.config.Twitch.Cookie)
	if ok {
		payload.Twitch.Name = twitch.DisplayName
		payload.Twitch.Email = twitch.Email
		payload.Twitch.LoggedIn = true
		payload.Twitch.ID = twitch.ID
	}
	dgg, ok := ur.getUserFromJWT(c, ur.config.Destinygg.Cookie)
	if ok {
		payload.Destinygg.Name = dgg.DisplayName
		payload.Destinygg.LoggedIn = true
		payload.Destinygg.ID = dgg.ID
	}
	c.HTML(http.StatusOK, "index.tmpl", payload)
}

// VerifyPayload ...
type VerifyPayload struct {
	UserID  string
	Name    string
	Email   string
	Valid   bool
	JWT     string
	Service string
	ID      string
}

func (ur *UnRustleLogs) verifyHandler(c *gin.Context) {
	payload := VerifyPayload{}
	if id := c.Query("id"); id != "" {
		id = strings.TrimSpace(id)
		// make sure the uuid is valid
		uid, err := uuid.Parse(id)
		if err != nil {
			c.HTML(http.StatusBadRequest, "verify.tmpl", payload)
			return
		}
		user, ok := ur.GetUser(uid.String())
		if !ok {
			c.HTML(http.StatusBadRequest, "verify.tmpl", payload)
			return
		}
		payload.UserID = user.UserID
		payload.Name = user.Name
		payload.Email = user.Email
		payload.Valid = true
		payload.Service = user.Service
		payload.ID = uid.String()
	}

	c.HTML(http.StatusOK, "verify.tmpl", payload)
}

func (ur *UnRustleLogs) getUserFromJWT(c *gin.Context, cookiename string) (*User, bool) {
	cookie, err := c.Cookie(cookiename)
	if err != nil {
		return nil, false
	}
	token, err := jwt.ParseWithClaims(cookie, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(ur.config.Server.JWTSecret), nil
	})
	if err != nil {
		logrus.Error(err)
		ur.deleteCookie(c, cookie)
		return nil, false
	}

	if claims, ok := token.Claims.(*jwtClaims); ok && token.Valid {
		// idk if i have to manually check if the jwt is expired or not
		// might be that .Valid is only true if it's not expired also
		now := time.Now()
		expires := time.Unix(claims.ExpiresAt, 0)
		if now.After(expires) {
			ur.deleteCookie(c, cookie)
			return nil, false
		}
		return ur.GetUser(claims.ID)
	}
	return nil, false
}

func (ur *UnRustleLogs) parseJWT(jwtString string) (*jwtClaims, bool) {
	token, err := jwt.ParseWithClaims(jwtString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(ur.config.Server.JWTSecret), nil
	})
	if err != nil {
		logrus.Error(err)
		return nil, false
	}

	if claims, ok := token.Claims.(*jwtClaims); ok && token.Valid {
		return claims, true
	}
	return nil, false
}

func (ur *UnRustleLogs) addDggState(s, verifier string) {
	ur.dggStateMutex.Lock()
	defer ur.dggStateMutex.Unlock()
	ur.dggStates[s] = &state{
		verifier: verifier,
		service:  TWITCHSERVICE,
		time:     time.Now().UTC(),
	}
	// delete dgg state after 5 minutes
	go func() {
		time.Sleep(time.Minute * 5)
		ur.deleteDggState(s)
	}()
}

func (ur *UnRustleLogs) hasDggState(state string) (string, bool) {
	ur.dggStateMutex.RLock()
	defer ur.dggStateMutex.RUnlock()
	s, ok := ur.dggStates[state]
	return s.verifier, ok
}

func (ur *UnRustleLogs) deleteDggState(state string) {
	ur.dggStateMutex.Lock()
	defer ur.dggStateMutex.Unlock()
	_, ok := ur.dggStates[state]
	if ok {
		logrus.Infof("deleting dgg state %s", state)
		delete(ur.dggStates, state)
	}
}

func (ur *UnRustleLogs) addTwitchState(s string) {
	ur.twitchStateMutex.Lock()
	defer ur.twitchStateMutex.Unlock()
	ur.twitchStates[s] = struct{}{}
	go func() {
		time.Sleep(time.Minute * 5)
		ur.deleteTwitchState(s)
	}()
}

func (ur *UnRustleLogs) hasTwitchState(state string) bool {
	ur.twitchStateMutex.RLock()
	defer ur.twitchStateMutex.RUnlock()
	_, ok := ur.twitchStates[state]
	return ok
}

func (ur *UnRustleLogs) deleteTwitchState(state string) {
	ur.twitchStateMutex.Lock()
	defer ur.twitchStateMutex.Unlock()
	_, ok := ur.twitchStates[state]
	if ok {
		logrus.Infof("deleting twitch state %s", state)
		delete(ur.twitchStates, state)
	}
}
