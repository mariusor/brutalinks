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

type backendConfig struct {
	Enabled bool
	Host    string
	Port    string
	User    string
	Pw      string
	Name    string
}

type Configuration struct {
	HostName                   string
	Port                       int
	APIURL                     string
	Name                       string
	Secure                     bool
	Env                        EnvType
	LogLevel                   log.Level
	DB                         backendConfig
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

const DefaultPort = 3000

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
	if port, _ := strconv.ParseInt(os.Getenv("PORT"), 10, 32); port > 0 {
		c.Port = int(port)
	} else {
		c.Port = DefaultPort
	}
	
	c.Secure, _ = strconv.ParseBool(os.Getenv("HTTPS"))

	//c.DB.Host = os.Getenv("DB_HOST")
	//c.DB.Pw = os.Getenv("DB_PASSWORD")
	//c.DB.Name = os.Getenv("DB_NAME")
	//c.DB.Port = os.Getenv("DB_PORT")
	//c.DB.User = os.Getenv("DB_USER")

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
