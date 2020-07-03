package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mariusor/littr.go/internal/log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Configuration struct {
	HostName                   string
	Name                       string
	ListenPort                 int
	ListenHost                 string
	APIURL                     string
	Secure                     bool
	Env                        EnvType
	LogLevel                   log.Level
	AdminContact               string
	AnonymousCommentingEnabled bool
	SessionsEnabled            bool
	VotingEnabled              bool
	DownvotingEnabled          bool
	UserCreatingEnabled        bool
	UserFollowingEnabled       bool
	ModerationEnabled          bool
	MaintenanceMode            bool
}

const DefaultListenPort = 3000
const DefaultListenHost = "localhost"

func Load(e EnvType) (*Configuration, error) {
	c := new(Configuration)
	configs := []string{
		".env",
	}
	if !ValidEnv(e) {
		env := os.Getenv("ENV")
		e = EnvType(strings.ToLower(env))
	}
	if ValidEnv(e) {
		c.Env = e
		configs = append(configs, fmt.Sprintf(".env.%s", c.Env))
	}

	lvl := os.Getenv("LOG_LEVEL")
	switch strings.ToLower(lvl) {
	case "trace":
		c.LogLevel = log.TraceLevel
	case "debug":
		c.LogLevel = log.DebugLevel
	case "warn":
		c.LogLevel = log.WarnLevel
	case "error":
		c.LogLevel = log.ErrorLevel
	case "info":
		fallthrough
	default:
		c.LogLevel = log.InfoLevel
	}

	for _, f := range configs {
		if err := godotenv.Overload(f); err != nil {
			return nil, err
		}
	}
	c.HostName = os.Getenv("HOSTNAME")
	c.Name = os.Getenv("NAME")
	if c.Name == "" {
		c.Name = c.HostName
	}
	c.ListenHost = os.Getenv("LISTEN_HOSTNAME")
	if c.ListenHost == "" {
		c.ListenHost = DefaultListenHost
	}
	if port, _ := strconv.ParseInt(os.Getenv("LISTEN_PORT"), 10, 32); port > 0 {
		c.ListenPort = int(port)
	} else {
		c.ListenPort = DefaultListenPort
	}

	c.Secure, _ = strconv.ParseBool(os.Getenv("HTTPS"))

	votingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_VOTING"))
	c.VotingEnabled = !votingDisabled
	if c.VotingEnabled {
		downvotingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_DOWNVOTING"))
		c.DownvotingEnabled = !downvotingDisabled
	}
	sessionsDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_SESSIONS"))
	c.SessionsEnabled = !sessionsDisabled
	userCreationDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_USER_CREATION"))
	c.UserCreatingEnabled = !userCreationDisabled
	// TODO(marius): this stopped working - as the anonymous user doesn't have a valid Outbox.
	anonymousCommentingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_ANONYMOUS_COMMENTING"))
	c.AnonymousCommentingEnabled = !anonymousCommentingDisabled
	userFollowingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_USER_FOLLOWING"))
	c.UserFollowingEnabled = !userFollowingDisabled
	moderationDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_MODERATION"))
	c.ModerationEnabled = !moderationDisabled
	c.AdminContact = os.Getenv("ADMIN_CONTACT")

	c.APIURL = os.Getenv("API_URL")

	return c, nil
}

func (c *Configuration) CheckUserCreatingEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.UserCreatingEnabled {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
		next.ServeHTTP(w, r)
	})
}
